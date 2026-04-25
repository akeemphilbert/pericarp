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

// NetSuite OAuth 2.0 host pattern templates. The placeholder is replaced with
// the normalized account ID. Auth runs on app.netsuite.com; token, revoke and
// userinfo run on suitetalk.api.netsuite.com.
//
// Reference: NetSuite Help > Integration > SuiteCloud Platform >
// "OAuth 2.0 for Integration"
// https://docs.oracle.com/en/cloud/saas/netsuite/ns-online-help/section_157771733782.html
const (
	netSuiteAuthURLTemplate     = "https://%s.app.netsuite.com/app/login/oauth2/authorize.nl"
	netSuiteTokenURLTemplate    = "https://%s.suitetalk.api.netsuite.com/services/rest/auth/oauth2/v1/token"
	netSuiteRevokeURLTemplate   = "https://%s.suitetalk.api.netsuite.com/services/rest/auth/oauth2/v1/revoke"
	netSuiteUserInfoURLTemplate = "https://%s.suitetalk.api.netsuite.com/services/rest/auth/oauth2/v1/userinfo"
)

// ErrNetSuiteIDTokenNotSupported is returned by NetSuite.ValidateIDToken
// because NetSuite's OAuth 2.0 implementation does not reliably issue
// OIDC-conformant ID tokens. Use Exchange to fetch user info from NetSuite's
// userinfo endpoint instead.
//
// Reference: NetSuite Help > Integration > SuiteCloud Platform >
// "OAuth 2.0 for Integration"
// https://docs.oracle.com/en/cloud/saas/netsuite/ns-online-help/section_157771733782.html
var ErrNetSuiteIDTokenNotSupported = errors.New("netsuite: OAuth 2.0 ID tokens are not supported; use Exchange to obtain user info")

// NetSuiteConfig holds the configuration for the NetSuite OAuth 2.0 provider.
//
// AccountID is required to derive the per-account hosts. Endpoint overrides
// take precedence over the derived URLs even when AccountID is set — that is
// the safety valve for sandboxes with non-standard hosts and for any future
// NetSuite endpoint change.
type NetSuiteConfig struct {
	ClientID     string
	ClientSecret string
	AccountID    string   // e.g. "1234567" (prod) or "1234567_SB1" (sandbox)
	Scopes       []string // defaults to ["rest_webservices"]

	// Endpoint overrides. When set, each takes precedence over the
	// account-derived URL. An empty string falls through to the derived URL.
	AuthEndpoint     string
	TokenEndpoint    string
	RevokeEndpoint   string
	UserInfoEndpoint string
}

// NetSuite implements the application.OAuthProvider interface for NetSuite OAuth 2.0.
type NetSuite struct {
	clientID         string
	clientSecret     string
	accountID        string
	scopes           []string
	authEndpoint     string
	tokenEndpoint    string
	revokeEndpoint   string
	userInfoEndpoint string
	httpClient       *http.Client
}

// NewNetSuite creates a new NetSuite OAuth provider from the given configuration.
// If no scopes are provided, it defaults to ["rest_webservices"] — the SuiteTalk
// REST scope that authorizes the userinfo endpoint Exchange calls after token issuance.
func NewNetSuite(config NetSuiteConfig) *NetSuite {
	scopes := config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"rest_webservices"}
	}

	return &NetSuite{
		clientID:         config.ClientID,
		clientSecret:     config.ClientSecret,
		accountID:        config.AccountID,
		scopes:           scopes,
		authEndpoint:     config.AuthEndpoint,
		tokenEndpoint:    config.TokenEndpoint,
		revokeEndpoint:   config.RevokeEndpoint,
		userInfoEndpoint: config.UserInfoEndpoint,
		httpClient:       &http.Client{Timeout: 30 * time.Second},
	}
}

// Name returns the provider identifier.
func (n *NetSuite) Name() string {
	return "netsuite"
}

// hostAccount normalizes the account ID for use in NetSuite hostnames:
// lowercase and replace underscores with dashes. Per NetSuite docs, sandbox
// accounts (e.g. "1234567_SB1") must use dashes in the URL host ("1234567-sb1").
func (n *NetSuite) hostAccount() string {
	return strings.ToLower(strings.ReplaceAll(n.accountID, "_", "-"))
}

func (n *NetSuite) authURL() string {
	if n.authEndpoint != "" {
		return n.authEndpoint
	}
	return fmt.Sprintf(netSuiteAuthURLTemplate, n.hostAccount())
}

func (n *NetSuite) tokenURL() string {
	if n.tokenEndpoint != "" {
		return n.tokenEndpoint
	}
	return fmt.Sprintf(netSuiteTokenURLTemplate, n.hostAccount())
}

func (n *NetSuite) revokeURL() string {
	if n.revokeEndpoint != "" {
		return n.revokeEndpoint
	}
	return fmt.Sprintf(netSuiteRevokeURLTemplate, n.hostAccount())
}

func (n *NetSuite) userInfoURL() string {
	if n.userInfoEndpoint != "" {
		return n.userInfoEndpoint
	}
	return fmt.Sprintf(netSuiteUserInfoURLTemplate, n.hostAccount())
}

// AuthCodeURL generates the NetSuite authorization URL with PKCE parameters.
func (n *NetSuite) AuthCodeURL(state string, codeChallenge string, nonce string, redirectURI string) string {
	params := url.Values{
		"client_id":             {n.clientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(n.scopes, " ")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"nonce":                 {nonce},
	}
	return n.authURL() + "?" + params.Encode()
}

type netSuiteTokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	IDToken          string `json:"id_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type netSuiteUserInfo struct {
	Sub               string `json:"sub"`
	Email             string `json:"email"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
}

// Exchange exchanges an authorization code for tokens and fetches user info.
func (n *NetSuite) Exchange(ctx context.Context, code string, codeVerifier string, redirectURI string) (*application.AuthResult, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
	}

	tokenResp, err := n.requestToken(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("netsuite: token exchange failed: %w", err)
	}

	userInfo, err := n.fetchUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("netsuite: failed to fetch user info: %w", err)
	}

	return &application.AuthResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		UserInfo:     *userInfo,
	}, nil
}

// RefreshToken refreshes an access token using a refresh token.
func (n *NetSuite) RefreshToken(ctx context.Context, refreshToken string) (*application.AuthResult, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	tokenResp, err := n.requestToken(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("netsuite: token refresh failed: %w", err)
	}

	// NetSuite may not return a new refresh token on refresh; preserve the original.
	if tokenResp.RefreshToken == "" {
		tokenResp.RefreshToken = refreshToken
	}

	userInfo, err := n.fetchUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("netsuite: failed to fetch user info after refresh: %w", err)
	}

	return &application.AuthResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		UserInfo:     *userInfo,
	}, nil
}

// RevokeToken revokes a token at NetSuite's revocation endpoint (RFC 7009).
// NetSuite mandates HTTP Basic on this endpoint; form-encoded
// client_id/client_secret is rejected.
func (n *NetSuite) RevokeToken(ctx context.Context, token string) error {
	data := url.Values{
		"token": {token},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.revokeURL(), strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("netsuite: failed to create revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(n.clientID, n.clientSecret)

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("netsuite: revoke request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("netsuite: revoke failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ValidateIDToken returns ErrNetSuiteIDTokenNotSupported because NetSuite's
// OAuth 2.0 implementation does not reliably issue OIDC-conformant ID tokens.
// Callers should use Exchange (which calls userinfo) to obtain user info.
//
// Reference: NetSuite Help > Integration > SuiteCloud Platform >
// "OAuth 2.0 for Integration"
// https://docs.oracle.com/en/cloud/saas/netsuite/ns-online-help/section_157771733782.html
func (n *NetSuite) ValidateIDToken(_ context.Context, _ string, _ string) (*application.UserInfo, error) {
	return nil, ErrNetSuiteIDTokenNotSupported
}

// requestToken POSTs to NetSuite's token endpoint. NetSuite expects client
// credentials via HTTP Basic, not in the request body. NetSuite can also
// return 200 with an OAuth 2.0 error body (RFC 6749 §5.2-style) — the empty
// AccessToken / non-empty Error guards below catch that case so callers don't
// proceed with a blank Bearer token.
func (n *NetSuite) requestToken(ctx context.Context, data url.Values) (*netSuiteTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.tokenURL(), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(n.clientID, n.clientSecret)

	resp, err := n.httpClient.Do(req)
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

	var tokenResp netSuiteTokenResponse
	if err = json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("token endpoint error: %s - %s", tokenResp.Error, tokenResp.ErrorDescription)
	}
	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("token response missing access_token")
	}

	return &tokenResp, nil
}

func (n *NetSuite) fetchUserInfo(ctx context.Context, accessToken string) (*application.UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, n.userInfoURL(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := n.httpClient.Do(req)
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

	var info netSuiteUserInfo
	if err = json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("failed to parse userinfo response: %w", err)
	}

	// Guard against silently issuing an identity with an empty ProviderUserID
	// when NetSuite returns a partial / non-standard userinfo payload.
	if info.Sub == "" {
		return nil, fmt.Errorf("userinfo response missing sub")
	}

	displayName := info.Name
	if displayName == "" {
		displayName = info.PreferredUsername
	}

	return &application.UserInfo{
		ProviderUserID: info.Sub,
		Email:          info.Email,
		DisplayName:    displayName,
		AvatarURL:      "",
		Provider:       "netsuite",
	}, nil
}
