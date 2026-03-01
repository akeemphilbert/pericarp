package providers

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
)

// Apple Sign In OAuth 2.0 / OIDC endpoint constants.
const (
	appleAuthEndpoint   = "https://appleid.apple.com/auth/authorize"
	appleTokenEndpoint  = "https://appleid.apple.com/auth/token"
	appleRevokeEndpoint = "https://appleid.apple.com/auth/revoke"
)

// AppleConfig holds configuration for the Apple Sign In OAuth provider.
type AppleConfig struct {
	ClientID   string   // The Services ID (e.g., "com.example.app.web")
	TeamID     string   // Apple Developer Team ID
	KeyID      string   // Key ID for the private key
	PrivateKey string   // PEM-encoded private key content (.p8 file contents)
	Scopes     []string // defaults to ["name", "email"]
}

// Apple implements the application.OAuthProvider interface for Apple Sign In OAuth 2.0 / OIDC.
type Apple struct {
	clientID   string
	teamID     string
	keyID      string
	privateKey string
	scopes     []string
	httpClient *http.Client
}

// NewApple creates a new Apple Sign In OAuth provider from the given configuration.
// If no scopes are provided, it defaults to ["name", "email"].
func NewApple(config AppleConfig) *Apple {
	scopes := config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"name", "email"}
	}

	return &Apple{
		clientID:   config.ClientID,
		teamID:     config.TeamID,
		keyID:      config.KeyID,
		privateKey: config.PrivateKey,
		scopes:     scopes,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Name returns the provider identifier.
func (a *Apple) Name() string {
	return "apple"
}

// AuthCodeURL generates the Apple authorization URL.
//
// Apple uses response_mode=form_post, meaning the authorization code is delivered
// via an HTTP POST to the redirect URI rather than as a query parameter.
func (a *Apple) AuthCodeURL(state string, _ string, nonce string, redirectURI string) string {
	params := url.Values{
		"client_id":     {a.clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {strings.Join(a.scopes, " ")},
		"state":         {state},
		"nonce":         {nonce},
		"response_mode": {"form_post"},
	}

	return appleAuthEndpoint + "?" + params.Encode()
}

// appleTokenResponse represents the JSON response from Apple's token endpoint.
type appleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// appleIDTokenClaims represents the JWT claims extracted from an Apple ID token.
type appleIDTokenClaims struct {
	Sub            string `json:"sub"`
	Email          string `json:"email"`
	EmailVerified  any    `json:"email_verified"` // Apple may return bool or string
	Nonce          string `json:"nonce"`
	NonceSupported bool   `json:"nonce_supported"`
	Iss            string `json:"iss"`
	Aud            string `json:"aud"`
	Exp            int64  `json:"exp"`
}

// Exchange exchanges an authorization code for tokens and extracts user info from the ID token.
//
// Apple does NOT support PKCE, so the codeVerifier parameter is ignored.
// User info is extracted from the ID token since Apple does not provide a separate userinfo endpoint.
// Apple only sends the user's name on the very first authorization via form_post body,
// so DisplayName and AvatarURL will be empty here.
func (a *Apple) Exchange(ctx context.Context, code string, _ string, redirectURI string) (*application.AuthResult, error) {
	clientSecret, err := a.generateClientSecret()
	if err != nil {
		return nil, fmt.Errorf("apple: failed to generate client secret: %w", err)
	}

	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {a.clientID},
		"client_secret": {clientSecret},
	}

	tokenResp, err := a.requestToken(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("apple: token exchange failed: %w", err)
	}

	userInfo, err := a.extractUserInfoFromIDToken(tokenResp.IDToken)
	if err != nil {
		return nil, fmt.Errorf("apple: failed to extract user info from ID token: %w", err)
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
//
// Apple refresh responses do NOT return a new ID token, so UserInfo will have
// minimal data. If an ID token is present, it will be decoded for user info.
func (a *Apple) RefreshToken(ctx context.Context, refreshToken string) (*application.AuthResult, error) {
	clientSecret, err := a.generateClientSecret()
	if err != nil {
		return nil, fmt.Errorf("apple: failed to generate client secret: %w", err)
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {a.clientID},
		"client_secret": {clientSecret},
	}

	tokenResp, err := a.requestToken(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("apple: token refresh failed: %w", err)
	}

	// Preserve the original refresh token if Apple doesn't return a new one.
	if tokenResp.RefreshToken == "" {
		tokenResp.RefreshToken = refreshToken
	}

	result := &application.AuthResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		IDToken:      tokenResp.IDToken,
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    tokenResp.ExpiresIn,
		UserInfo: application.UserInfo{
			Provider: "apple",
		},
	}

	// If an ID token is present, decode it for user info.
	if tokenResp.IDToken != "" {
		userInfo, decodeErr := a.extractUserInfoFromIDToken(tokenResp.IDToken)
		if decodeErr == nil {
			result.UserInfo = *userInfo
		}
	}

	return result, nil
}

// RevokeToken revokes a token at Apple's revocation endpoint.
func (a *Apple) RevokeToken(ctx context.Context, token string) error {
	clientSecret, err := a.generateClientSecret()
	if err != nil {
		return fmt.Errorf("apple: failed to generate client secret: %w", err)
	}

	data := url.Values{
		"client_id":       {a.clientID},
		"client_secret":   {clientSecret},
		"token":           {token},
		"token_type_hint": {"access_token"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, appleRevokeEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("apple: failed to create revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("apple: revoke request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("apple: revoke failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ValidateIDToken decodes and validates an Apple ID token, returning the user info from claims.
//
// NOTE: This implementation performs basic structural validation of the JWT payload
// (issuer, audience, expiry, nonce) but does NOT verify the JWT signature.
// Production deployments should verify the JWT signature using Apple's JWKS endpoint
// at https://appleid.apple.com/auth/keys to ensure the token has not been tampered with.
func (a *Apple) ValidateIDToken(_ context.Context, idToken string, nonce string) (*application.UserInfo, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("apple: invalid ID token format: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("apple: failed to decode ID token payload: %w", err)
	}

	var claims appleIDTokenClaims
	if err = json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("apple: failed to parse ID token claims: %w", err)
	}

	// Validate issuer.
	if claims.Iss != "https://appleid.apple.com" {
		return nil, fmt.Errorf("apple: invalid ID token issuer: %s", claims.Iss)
	}

	// Validate audience.
	if claims.Aud != a.clientID {
		return nil, fmt.Errorf("apple: invalid ID token audience: expected %s, got %s", a.clientID, claims.Aud)
	}

	// Validate expiration.
	if time.Now().Unix() > claims.Exp {
		return nil, fmt.Errorf("apple: ID token has expired")
	}

	// Validate nonce if the token supports it.
	if claims.NonceSupported && claims.Nonce != nonce {
		return nil, fmt.Errorf("apple: ID token nonce mismatch")
	}

	return &application.UserInfo{
		ProviderUserID: claims.Sub,
		Email:          claims.Email,
		DisplayName:    "",
		AvatarURL:      "",
		Provider:       "apple",
	}, nil
}

// generateClientSecret creates a JWT client secret for Apple Sign In.
//
// Apple requires a signed JWT instead of a static client_secret. The JWT is signed
// with the ES256 algorithm using the private key associated with the Key ID.
//
// JWT structure:
//   - Header: {"alg": "ES256", "kid": <keyID>}
//   - Claims: iss=teamID, iat=now, exp=now+6months, aud="https://appleid.apple.com", sub=clientID
func (a *Apple) generateClientSecret() (string, error) {
	// Parse the PEM-encoded private key.
	block, _ := pem.Decode([]byte(a.privateKey))
	if block == nil {
		return "", fmt.Errorf("failed to decode PEM block from private key")
	}

	parsedKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse PKCS8 private key: %w", err)
	}

	ecKey, ok := parsedKey.(*ecdsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("private key is not an ECDSA key")
	}

	// Build the JWT header.
	header := map[string]string{
		"alg": "ES256",
		"kid": a.keyID,
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWT header: %w", err)
	}

	// Build the JWT claims.
	now := time.Now()
	claims := map[string]any{
		"iss": a.teamID,
		"iat": now.Unix(),
		"exp": now.Add(6 * 30 * 24 * time.Hour).Unix(), // ~6 months
		"aud": "https://appleid.apple.com",
		"sub": a.clientID,
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JWT claims: %w", err)
	}

	// Encode header and claims as base64url.
	headerEncoded := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsEncoded := base64.RawURLEncoding.EncodeToString(claimsJSON)

	// Create the signing input.
	signingInput := headerEncoded + "." + claimsEncoded

	// Sign with ES256 (ECDSA using P-256 and SHA-256).
	hash := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, ecKey, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	// Encode the signature in JWS compact format: r || s, each padded to 32 bytes.
	curveBits := ecKey.Curve.Params().BitSize
	keyBytes := curveBits / 8
	if curveBits%8 > 0 {
		keyBytes++
	}

	rBytes := r.Bytes()
	sBytes := s.Bytes()

	// Pad r and s to the correct length.
	sigBytes := make([]byte, 2*keyBytes)
	copy(sigBytes[keyBytes-len(rBytes):keyBytes], rBytes)
	copy(sigBytes[2*keyBytes-len(sBytes):], sBytes)

	signatureEncoded := base64.RawURLEncoding.EncodeToString(sigBytes)

	return signingInput + "." + signatureEncoded, nil
}

// requestToken performs a POST to Apple's token endpoint and parses the response.
func (a *Apple) requestToken(ctx context.Context, data url.Values) (*appleTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, appleTokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp appleTokenResponse
	if err = json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	return &tokenResp, nil
}

// extractUserInfoFromIDToken decodes the JWT payload from an Apple ID token
// and returns the user info. Apple does not provide name or avatar in the ID token.
func (a *Apple) extractUserInfoFromIDToken(idToken string) (*application.UserInfo, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid ID token format: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode ID token payload: %w", err)
	}

	var claims struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
	}
	if err = json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("failed to parse ID token claims: %w", err)
	}

	return &application.UserInfo{
		ProviderUserID: claims.Sub,
		Email:          claims.Email,
		DisplayName:    "",
		AvatarURL:      "",
		Provider:       "apple",
	}, nil
}
