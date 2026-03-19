package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
)

// Google OAuth 2.0 / OIDC endpoint constants.
const (
	googleAuthEndpoint     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenEndpoint    = "https://oauth2.googleapis.com/token"
	googleRevokeEndpoint   = "https://oauth2.googleapis.com/revoke"
	googleUserInfoEndpoint = "https://www.googleapis.com/oauth2/v3/userinfo"
)

// GoogleConfig holds configuration for the Google OAuth provider.
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	Scopes       []string // defaults to ["openid", "email", "profile"]
}

// Google implements the application.OAuthProvider interface for Google OAuth 2.0 / OIDC.
type Google struct {
	clientID     string
	clientSecret string
	scopes       []string
	httpClient   *http.Client
}

// NewGoogle creates a new Google OAuth provider from the given configuration.
// If no scopes are provided, it defaults to ["openid", "email", "profile"].
func NewGoogle(config GoogleConfig) *Google {
	scopes := config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "email", "profile"}
	}

	return &Google{
		clientID:     config.ClientID,
		clientSecret: config.ClientSecret,
		scopes:       scopes,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Name returns the provider identifier.
func (g *Google) Name() string {
	return "google"
}

// AuthCodeURL generates the Google authorization URL with PKCE parameters.
func (g *Google) AuthCodeURL(state string, codeChallenge string, nonce string, redirectURI string) string {
	params := url.Values{
		"client_id":             {g.clientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(g.scopes, " ")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"nonce":                 {nonce},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
	}

	return googleAuthEndpoint + "?" + params.Encode()
}

// tokenResponse represents the JSON response from Google's token endpoint.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// googleUserInfo represents the JSON response from Google's userinfo endpoint.
type googleUserInfo struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// Exchange exchanges an authorization code for tokens and fetches user info.
func (g *Google) Exchange(ctx context.Context, code string, codeVerifier string, redirectURI string) (*application.AuthResult, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"code_verifier": {codeVerifier},
	}

	tokenResp, err := g.requestToken(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("google: token exchange failed: %w", err)
	}

	userInfo, err := g.fetchUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("google: failed to fetch user info: %w", err)
	}

	return &application.AuthResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		UserInfo: application.UserInfo{
			ProviderUserID: userInfo.Sub,
			Email:          userInfo.Email,
			DisplayName:    userInfo.Name,
			AvatarURL:      userInfo.Picture,
			Provider:       "google",
		},
	}, nil
}

// RefreshToken refreshes an access token using a refresh token and fetches updated user info.
func (g *Google) RefreshToken(ctx context.Context, refreshToken string) (*application.AuthResult, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
	}

	tokenResp, err := g.requestToken(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("google: token refresh failed: %w", err)
	}

	// Google may not return a new refresh token on refresh; preserve the original.
	if tokenResp.RefreshToken == "" {
		tokenResp.RefreshToken = refreshToken
	}

	userInfo, err := g.fetchUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("google: failed to fetch user info after refresh: %w", err)
	}

	return &application.AuthResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		UserInfo: application.UserInfo{
			ProviderUserID: userInfo.Sub,
			Email:          userInfo.Email,
			DisplayName:    userInfo.Name,
			AvatarURL:      userInfo.Picture,
			Provider:       "google",
		},
	}, nil
}

// RevokeToken revokes a token at Google's revocation endpoint.
func (g *Google) RevokeToken(ctx context.Context, token string) error {
	data := url.Values{
		"token": {token},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleRevokeEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("google: failed to create revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("google: revoke request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("google: revoke failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// idTokenClaims represents the JWT claims extracted from a Google ID token.
type idTokenClaims struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
	Nonce   string `json:"nonce"`
	Iss     string `json:"iss"`
	Aud     string `json:"aud"`
	Exp     int64  `json:"exp"`
}

// ValidateIDToken decodes and validates a Google ID token, returning the user info from claims.
//
// NOTE: This implementation performs basic structural validation of the JWT payload
// (issuer, audience, expiry, nonce) but does NOT verify the JWT signature.
// Production deployments should verify the JWT signature using Google's JWKS endpoint
// at https://www.googleapis.com/oauth2/v3/certs to ensure the token has not been tampered with.
func (g *Google) ValidateIDToken(_ context.Context, idToken string, nonce string) (*application.UserInfo, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("google: invalid ID token format: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("google: failed to decode ID token payload: %w", err)
	}

	var claims idTokenClaims
	if err = json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("google: failed to parse ID token claims: %w", err)
	}

	// Validate issuer.
	if claims.Iss != "https://accounts.google.com" && claims.Iss != "accounts.google.com" {
		return nil, fmt.Errorf("google: invalid ID token issuer: %s", claims.Iss)
	}

	// Validate audience.
	if claims.Aud != g.clientID {
		return nil, fmt.Errorf("google: invalid ID token audience: expected %s, got %s", g.clientID, claims.Aud)
	}

	// Validate expiration.
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("google: ID token has expired")
	}

	// Validate nonce.
	if claims.Nonce != nonce {
		return nil, fmt.Errorf("google: ID token nonce mismatch")
	}

	return &application.UserInfo{
		ProviderUserID: claims.Sub,
		Email:          claims.Email,
		DisplayName:    claims.Name,
		AvatarURL:      claims.Picture,
		Provider:       "google",
	}, nil
}

// requestToken performs a POST to Google's token endpoint and parses the response.
func (g *Google) requestToken(ctx context.Context, data url.Values) (*tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
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

	var tokenResp tokenResponse
	if err = json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}

// fetchUserInfo retrieves user information from Google's userinfo endpoint using the access token.
func (g *Google) fetchUserInfo(ctx context.Context, accessToken string) (*googleUserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserInfoEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := g.httpClient.Do(req)
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

	var userInfo googleUserInfo
	if err = json.Unmarshal(body, &userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse userinfo response: %w", err)
	}

	return &userInfo, nil
}
