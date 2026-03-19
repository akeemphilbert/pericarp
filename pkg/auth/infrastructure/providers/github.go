package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
)

const (
	githubAuthURL       = "https://github.com/login/oauth/authorize"
	githubTokenURL      = "https://github.com/login/oauth/access_token"
	githubUserURL       = "https://api.github.com/user"
	githubUserEmailsURL = "https://api.github.com/user/emails"
	githubRevokeURL     = "https://api.github.com/applications/%s/token"
)

// GitHubConfig holds configuration for the GitHub OAuth provider.
type GitHubConfig struct {
	ClientID     string
	ClientSecret string
	Scopes       []string // defaults to ["read:user", "user:email"]
}

// GitHub implements the application.OAuthProvider interface for GitHub OAuth.
type GitHub struct {
	clientID     string
	clientSecret string
	scopes       []string
	httpClient   *http.Client
}

// NewGitHub creates a new GitHub OAuth provider from the given configuration.
func NewGitHub(config GitHubConfig) *GitHub {
	scopes := config.Scopes
	if len(scopes) == 0 {
		scopes = []string{"read:user", "user:email"}
	}

	return &GitHub{
		clientID:     config.ClientID,
		clientSecret: config.ClientSecret,
		scopes:       scopes,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the provider identifier.
func (g *GitHub) Name() string {
	return "github"
}

// AuthCodeURL generates the GitHub authorization URL with the given parameters.
func (g *GitHub) AuthCodeURL(state string, codeChallenge string, nonce string, redirectURI string) string {
	params := url.Values{}
	params.Set("client_id", g.clientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", strings.Join(g.scopes, " "))
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("nonce", nonce)
	params.Set("allow_signup", "true")

	return githubAuthURL + "?" + params.Encode()
}

// Exchange exchanges an authorization code for tokens and fetches user info.
func (g *GitHub) Exchange(ctx context.Context, code string, codeVerifier string, redirectURI string) (*application.AuthResult, error) {
	// Build the token exchange request body.
	data := url.Values{}
	data.Set("client_id", g.clientID)
	data.Set("client_secret", g.clientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("github: failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: token request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("github: failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err = json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("github: failed to parse token response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("github: token exchange error: %s - %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("github: token response missing access_token")
	}

	// Fetch user info from GitHub API.
	userInfo, err := g.fetchUserInfo(ctx, tokenResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("github: failed to fetch user info: %w", err)
	}

	return &application.AuthResult{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: "",
		IDToken:      "",
		TokenType:    tokenResp.TokenType,
		ExpiresIn:    0,
		UserInfo:     *userInfo,
	}, nil
}

// RefreshToken is not supported by GitHub's standard OAuth flow.
func (g *GitHub) RefreshToken(_ context.Context, _ string) (*application.AuthResult, error) {
	return nil, fmt.Errorf("github: refresh tokens are not supported")
}

// RevokeToken revokes an access token via the GitHub Applications API.
func (g *GitHub) RevokeToken(ctx context.Context, token string) error {
	revokeURL := fmt.Sprintf(githubRevokeURL, g.clientID)

	requestBody, err := json.Marshal(map[string]string{
		"access_token": token,
	})
	if err != nil {
		return fmt.Errorf("github: failed to marshal revoke request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, revokeURL, bytes.NewReader(requestBody))
	if err != nil {
		return fmt.Errorf("github: failed to create revoke request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "pericarp-auth")
	req.SetBasicAuth(g.clientID, g.clientSecret)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("github: revoke request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Drain the response body to allow connection reuse.
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	return fmt.Errorf("github: revoke endpoint returned status %d", resp.StatusCode)
}

// ValidateIDToken is not supported by GitHub as it does not issue OIDC ID tokens.
func (g *GitHub) ValidateIDToken(_ context.Context, _ string, _ string) (*application.UserInfo, error) {
	return nil, fmt.Errorf("github: ID tokens are not supported, use Exchange to get user info")
}

// fetchUserInfo retrieves user profile and email information from the GitHub API.
func (g *GitHub) fetchUserInfo(ctx context.Context, accessToken string) (*application.UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubUserURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "pericarp-auth")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("user request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read user response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("user endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var ghUser struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}
	if err = json.Unmarshal(body, &ghUser); err != nil {
		return nil, fmt.Errorf("failed to parse user response: %w", err)
	}

	displayName := ghUser.Name
	if displayName == "" {
		displayName = ghUser.Login
	}

	email := ghUser.Email
	if email == "" {
		email, err = g.fetchPrimaryEmail(ctx, accessToken)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch primary email: %w", err)
		}
	}

	return &application.UserInfo{
		ProviderUserID: strconv.FormatInt(ghUser.ID, 10),
		Email:          email,
		DisplayName:    displayName,
		AvatarURL:      ghUser.AvatarURL,
		Provider:       "github",
	}, nil
}

// fetchPrimaryEmail retrieves the primary verified email from the GitHub user emails API.
func (g *GitHub) fetchPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubUserEmailsURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create emails request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "pericarp-auth")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("emails request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read emails response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("emails endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err = json.Unmarshal(body, &emails); err != nil {
		return "", fmt.Errorf("failed to parse emails response: %w", err)
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}

	return "", fmt.Errorf("no primary verified email found")
}
