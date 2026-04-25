package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
)

// Facebook Graph API v18.0 endpoints.
const (
	facebookAuthEndpoint     = "https://www.facebook.com/v18.0/dialog/oauth"
	facebookTokenEndpoint    = "https://graph.facebook.com/v18.0/oauth/access_token"
	facebookUserInfoEndpoint = "https://graph.facebook.com/v18.0/me"
	facebookPermissionsPath  = "/v18.0/me/permissions"
	facebookGraphHost        = "https://graph.facebook.com"
)

// ErrFacebookIDTokenUnsupported indicates that Facebook's standard Login flow
// does not issue OIDC ID tokens. Callers should resolve user identity via
// Exchange (which calls the Graph API) instead of relying on ValidateIDToken.
var ErrFacebookIDTokenUnsupported = errors.New("facebook: ID tokens are not supported by Facebook Login; use Exchange to resolve user info")

// FacebookConfig holds configuration for the Facebook OAuth provider.
//
// Facebook Login supports PKCE on Login for Business and consumer Login flows;
// this provider always emits S256 code challenges. Refresh tokens are not part
// of Facebook's user-access-token model — RefreshToken returns
// application.ErrTokenRefreshFailed by design.
type FacebookConfig struct {
	ClientID     string
	ClientSecret string
	Scopes       []string // defaults to ["email", "public_profile"]
}

// Facebook implements application.OAuthProvider for Facebook Login.
type Facebook struct {
	clientID         string
	clientSecret     string
	scopes           []string
	httpClient       *http.Client
	authEndpoint     string
	tokenEndpoint    string
	userInfoEndpoint string
	graphHost        string
}

// NewFacebook creates a new Facebook OAuth provider from the given configuration.
// If no scopes are provided, it defaults to ["email", "public_profile"].
func NewFacebook(config FacebookConfig) *Facebook {
	scopes := config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"email", "public_profile"}
	}

	return &Facebook{
		clientID:         config.ClientID,
		clientSecret:     config.ClientSecret,
		scopes:           scopes,
		httpClient:       &http.Client{Timeout: 30 * time.Second},
		authEndpoint:     facebookAuthEndpoint,
		tokenEndpoint:    facebookTokenEndpoint,
		userInfoEndpoint: facebookUserInfoEndpoint,
		graphHost:        facebookGraphHost,
	}
}

// Name returns the provider identifier.
func (f *Facebook) Name() string {
	return "facebook"
}

// AuthCodeURL generates the Facebook OAuth dialog URL with PKCE parameters.
func (f *Facebook) AuthCodeURL(state string, codeChallenge string, _ string, redirectURI string) string {
	params := url.Values{
		"client_id":             {f.clientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(f.scopes, ",")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}

	return f.authEndpoint + "?" + params.Encode()
}

// facebookTokenResponse models Facebook's token-endpoint JSON shape.
type facebookTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// facebookUserInfo models the Graph /me response shape.
type facebookUserInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Picture struct {
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	} `json:"picture"`
}

// Exchange swaps an authorization code for an access token and resolves user
// info via the Graph /me endpoint. Facebook does not issue refresh or ID tokens
// in this flow, so those fields on the AuthResult are left empty.
func (f *Facebook) Exchange(ctx context.Context, code string, codeVerifier string, redirectURI string) (*application.AuthResult, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {f.clientID},
		"client_secret": {f.clientSecret},
		"code_verifier": {codeVerifier},
	}

	tokenResp, err := f.requestToken(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("facebook: token exchange failed: %w", err)
	}

	userInfo, err := f.fetchUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("facebook: failed to fetch user info: %w", err)
	}

	return &application.AuthResult{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenResp.TokenType,
		ExpiresIn:   tokenResp.ExpiresIn,
		UserInfo: application.UserInfo{
			ProviderUserID: userInfo.ID,
			Email:          userInfo.Email,
			DisplayName:    userInfo.Name,
			AvatarURL:      userInfo.Picture.Data.URL,
			Provider:       "facebook",
		},
	}, nil
}

// RefreshToken returns application.ErrTokenRefreshFailed because Facebook
// user-access tokens are not refreshable via the standard refresh_token grant.
// Long-lived tokens are obtained server-side via Facebook's
// fb_exchange_token flow, which is intentionally outside this interface.
func (f *Facebook) RefreshToken(_ context.Context, _ string) (*application.AuthResult, error) {
	return nil, application.ErrTokenRefreshFailed
}

// RevokeToken revokes the user's app permissions via the Graph API.
//
// Facebook revokes by DELETE-ing /me/permissions with the access token as the
// caller's bearer credential; this revokes ALL permissions granted to the app
// for that user, which matches the standard OAuth "revoke" semantics.
func (f *Facebook) RevokeToken(ctx context.Context, token string) error {
	revokeURL := f.graphHost + facebookPermissionsPath + "?access_token=" + url.QueryEscape(token)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, revokeURL, nil)
	if err != nil {
		return fmt.Errorf("facebook: failed to create revoke request: %w", err)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("facebook: revoke request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("facebook: revoke failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Graph returns {"success": true} on success; treat anything else as an error.
	var result struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &result); err == nil && !result.Success {
		return fmt.Errorf("facebook: revoke returned success=false: %s", string(body))
	}
	return nil
}

// ValidateIDToken returns ErrFacebookIDTokenUnsupported. Standard Facebook
// Login does not issue OIDC ID tokens; Limited Login is a separate, mobile-SDK
// product. Resolve identity via Exchange.
func (f *Facebook) ValidateIDToken(_ context.Context, _ string, _ string) (*application.UserInfo, error) {
	return nil, ErrFacebookIDTokenUnsupported
}

// requestToken posts to the Facebook token endpoint and parses the response.
func (f *Facebook) requestToken(ctx context.Context, data url.Values) (*facebookTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp facebookTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token: %s", string(body))
	}
	return &tokenResp, nil
}

// fetchUserInfo calls Graph /me with the requested fields.
func (f *Facebook) fetchUserInfo(ctx context.Context, accessToken string) (*facebookUserInfo, error) {
	endpoint := f.userInfoEndpoint + "?" + url.Values{
		"fields":       {"id,name,email,picture"},
		"access_token": {accessToken},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create userinfo request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read userinfo response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo request returned status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo facebookUserInfo
	if err := json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse userinfo response: %w", err)
	}
	if userInfo.ID == "" {
		return nil, fmt.Errorf("userinfo response missing id: %s", string(body))
	}
	return &userInfo, nil
}
