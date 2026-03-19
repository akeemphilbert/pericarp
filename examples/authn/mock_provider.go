package main

import (
	"context"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
)

// MockOAuthProvider simulates an OAuth provider with deterministic responses.
type MockOAuthProvider struct {
	providerName string
}

func NewMockOAuthProvider(name string) *MockOAuthProvider {
	return &MockOAuthProvider{providerName: name}
}

func (m *MockOAuthProvider) Name() string { return m.providerName }

func (m *MockOAuthProvider) AuthCodeURL(state, codeChallenge, nonce, redirectURI string) string {
	return "https://" + m.providerName + ".example.com/auth?state=" + state
}

func (m *MockOAuthProvider) Exchange(_ context.Context, code, codeVerifier, redirectURI string) (*application.AuthResult, error) {
	return &application.AuthResult{
		AccessToken:  "access-token-" + code,
		RefreshToken: "refresh-token-" + code,
		IDToken:      "id-token-" + code,
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		UserInfo: application.UserInfo{
			ProviderUserID: "provider-user-42",
			Email:          "alice@example.com",
			DisplayName:    "Alice Example",
			Provider:       m.providerName,
		},
	}, nil
}

func (m *MockOAuthProvider) RefreshToken(_ context.Context, refreshToken string) (*application.AuthResult, error) {
	return &application.AuthResult{
		AccessToken:  "new-access-token",
		RefreshToken: "new-refresh-token",
		TokenType:    "Bearer",
		ExpiresIn:    3600,
	}, nil
}

func (m *MockOAuthProvider) RevokeToken(_ context.Context, token string) error {
	return nil
}

func (m *MockOAuthProvider) ValidateIDToken(_ context.Context, idToken, nonce string) (*application.UserInfo, error) {
	return &application.UserInfo{
		ProviderUserID: "provider-user-42",
		Email:          "alice@example.com",
		DisplayName:    "Alice Example",
		Provider:       m.providerName,
	}, nil
}
