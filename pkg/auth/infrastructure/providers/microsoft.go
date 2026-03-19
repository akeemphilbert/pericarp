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

// MicrosoftConfig holds the configuration for the Microsoft (Entra ID / Azure AD) OAuth provider.
type MicrosoftConfig struct {
	ClientID     string
	ClientSecret string
	TenantID     string   // defaults to "common" for multi-tenant
	Scopes       []string // defaults to ["openid", "email", "profile", "offline_access"]
}

// Microsoft implements the application.OAuthProvider interface for Microsoft identity platform v2.0
// (Entra ID / Azure AD) using OAuth 2.0 and OpenID Connect.
type Microsoft struct {
	clientID     string
	clientSecret string
	tenantID     string
	scopes       []string
	httpClient   *http.Client
}

// NewMicrosoft creates a new Microsoft OAuth provider from the given configuration.
func NewMicrosoft(config MicrosoftConfig) *Microsoft {
	tenantID := config.TenantID
	if tenantID == "" {
		tenantID = "common"
	}

	scopes := config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "email", "profile", "offline_access"}
	}

	return &Microsoft{
		clientID:     config.ClientID,
		clientSecret: config.ClientSecret,
		tenantID:     tenantID,
		scopes:       scopes,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the provider identifier.
func (m *Microsoft) Name() string {
	return "microsoft"
}

// authURL returns the Microsoft OAuth 2.0 authorization endpoint for the configured tenant.
func (m *Microsoft) authURL() string {
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", m.tenantID)
}

// tokenURL returns the Microsoft OAuth 2.0 token endpoint for the configured tenant.
func (m *Microsoft) tokenURL() string {
	return fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", m.tenantID)
}

// AuthCodeURL generates the authorization URL with PKCE parameters.
func (m *Microsoft) AuthCodeURL(state string, codeChallenge string, nonce string, redirectURI string) string {
	params := url.Values{
		"client_id":             {m.clientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(m.scopes, " ")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"nonce":                 {nonce},
		"response_mode":         {"query"},
	}

	return m.authURL() + "?" + params.Encode()
}

// microsoftTokenResponse represents the JSON response from the Microsoft token endpoint.
type microsoftTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// microsoftUserResponse represents the JSON response from the Microsoft Graph /me endpoint.
type microsoftUserResponse struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
}

// Exchange exchanges an authorization code for tokens.
func (m *Microsoft) Exchange(ctx context.Context, code string, codeVerifier string, redirectURI string) (*application.AuthResult, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {m.clientID},
		"client_secret": {m.clientSecret},
		"code_verifier": {codeVerifier},
	}

	tokenResp, err := m.requestToken(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("microsoft: token exchange failed: %w", err)
	}

	userInfo, err := m.fetchUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("microsoft: failed to fetch user info: %w", err)
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
func (m *Microsoft) RefreshToken(ctx context.Context, refreshToken string) (*application.AuthResult, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {m.clientID},
		"client_secret": {m.clientSecret},
		"scope":         {strings.Join(m.scopes, " ")},
	}

	tokenResp, err := m.requestToken(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("microsoft: token refresh failed: %w", err)
	}

	userInfo, err := m.fetchUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("microsoft: failed to fetch user info: %w", err)
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

// RevokeToken attempts to revoke a token. Microsoft's v2.0 identity platform does not support
// a standard token revocation endpoint.
func (m *Microsoft) RevokeToken(_ context.Context, _ string) error {
	return fmt.Errorf("microsoft: token revocation is not supported by Microsoft identity platform v2.0")
}

// microsoftIDTokenClaims represents the claims in a Microsoft ID token JWT payload.
type microsoftIDTokenClaims struct {
	Sub               string `json:"sub"`
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
	Name              string `json:"name"`
	Nonce             string `json:"nonce"`
	Issuer            string `json:"iss"`
	Audience          string `json:"aud"`
	Expiry            int64  `json:"exp"`
	TenantID          string `json:"tid"`
}

// ValidateIDToken validates the ID token and extracts user claims.
//
// NOTE: In production, the JWT signature should be verified using Microsoft's JWKS endpoint
// (https://login.microsoftonline.com/{tenantID}/discovery/v2.0/keys). This implementation
// only decodes and validates claims without cryptographic signature verification.
func (m *Microsoft) ValidateIDToken(_ context.Context, idToken string, nonce string) (*application.UserInfo, error) {
	claims, err := m.decodeIDToken(idToken)
	if err != nil {
		return nil, fmt.Errorf("microsoft: failed to decode ID token: %w", err)
	}

	// Validate audience
	if claims.Audience != m.clientID {
		return nil, fmt.Errorf("microsoft: invalid audience claim: expected %s, got %s", m.clientID, claims.Audience)
	}

	// Validate expiry
	if time.Now().Unix() > claims.Expiry {
		return nil, fmt.Errorf("microsoft: ID token has expired")
	}

	// Validate nonce
	if claims.Nonce != nonce {
		return nil, fmt.Errorf("microsoft: invalid nonce claim: expected %s, got %s", nonce, claims.Nonce)
	}

	// Validate issuer
	if !strings.Contains(claims.Issuer, "login.microsoftonline.com") {
		return nil, fmt.Errorf("microsoft: invalid issuer claim: %s", claims.Issuer)
	}

	// Determine email: prefer email claim, fall back to preferred_username
	email := claims.Email
	if email == "" {
		email = claims.PreferredUsername
	}

	return &application.UserInfo{
		ProviderUserID: claims.Sub,
		Email:          email,
		DisplayName:    claims.Name,
		AvatarURL:      "",
		Provider:       "microsoft",
	}, nil
}

// requestToken performs a POST request to the Microsoft token endpoint and decodes the response.
func (m *Microsoft) requestToken(ctx context.Context, data url.Values) (*microsoftTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.tokenURL(), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp microsoftTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}

// fetchUserInfo retrieves user information from the Microsoft Graph API using a Bearer access token.
func (m *Microsoft) fetchUserInfo(ctx context.Context, accessToken string) (*application.UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://graph.microsoft.com/v1.0/me", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("user info request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read user info response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("user info endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var userResp microsoftUserResponse
	if err := json.Unmarshal(body, &userResp); err != nil {
		return nil, fmt.Errorf("failed to parse user info response: %w", err)
	}

	// Use mail if available, fall back to userPrincipalName
	email := userResp.Mail
	if email == "" {
		email = userResp.UserPrincipalName
	}

	return &application.UserInfo{
		ProviderUserID: userResp.ID,
		Email:          email,
		DisplayName:    userResp.DisplayName,
		AvatarURL:      "",
		Provider:       "microsoft",
	}, nil
}

// decodeIDToken decodes the JWT payload from a Microsoft ID token without verifying the signature.
func (m *Microsoft) decodeIDToken(idToken string) (*microsoftIDTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT format: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to base64url-decode JWT payload: %w", err)
	}

	var claims microsoftIDTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse JWT claims: %w", err)
	}

	return &claims, nil
}
