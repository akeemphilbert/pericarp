package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

func TestAuthenticationFlow_FullLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	result, err := RunAuthenticationFlow(ctx)
	if err != nil {
		t.Fatalf("RunAuthenticationFlow() error: %v", err)
	}

	if result.AuthRequest == nil {
		t.Fatal("expected non-nil AuthRequest")
	}
	if result.AuthRequest.AuthURL == "" {
		t.Error("expected non-empty AuthURL")
	}
	if result.AuthRequest.State == "" {
		t.Error("expected non-empty State")
	}
	if result.AuthRequest.CodeVerifier == "" {
		t.Error("expected non-empty CodeVerifier")
	}

	if result.AuthResult == nil {
		t.Fatal("expected non-nil AuthResult")
	}
	if result.AuthResult.AccessToken == "" {
		t.Error("expected non-empty AccessToken")
	}
	if result.AuthResult.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want %q", result.AuthResult.TokenType, "Bearer")
	}

	if result.AgentID == "" {
		t.Error("expected non-empty AgentID")
	}
	if result.CredentialID == "" {
		t.Error("expected non-empty CredentialID")
	}
	if result.AccountID == "" {
		t.Error("expected non-empty AccountID")
	}

	if result.SessionID == "" {
		t.Error("expected non-empty SessionID")
	}
	if result.SessionInfo == nil {
		t.Fatal("expected non-nil SessionInfo")
	}
	if result.SessionInfo.AgentID != result.AgentID {
		t.Errorf("SessionInfo.AgentID = %q, want %q", result.SessionInfo.AgentID, result.AgentID)
	}

	if result.Token == "" {
		t.Error("expected non-empty JWT token")
	}
	if result.Claims == nil {
		t.Fatal("expected non-nil Claims")
	}
	if result.Claims.AgentID != result.AgentID {
		t.Errorf("Claims.AgentID = %q, want %q", result.Claims.AgentID, result.AgentID)
	}
	if result.Claims.ActiveAccountID != result.AccountID {
		t.Errorf("Claims.ActiveAccountID = %q, want %q", result.Claims.ActiveAccountID, result.AccountID)
	}
}

func TestAuthenticationFlow_ExistingAgent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Pre-populate repos
	agents := NewMemoryAgentRepository()
	credentials := NewMemoryCredentialRepository()
	accounts := NewMemoryAccountRepository()
	sessions := NewMemorySessionRepository()

	agent, err := new(entities.Agent).With("agent-existing", "Alice", entities.AgentTypePerson)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	_ = agents.Save(ctx, agent)

	cred, err := new(entities.Credential).With("cred-existing", "agent-existing", "mock-idp", "provider-user-42", "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("failed to create credential: %v", err)
	}
	_ = credentials.Save(ctx, cred)

	personalAccount, err := new(entities.Account).With("account-existing", "Alice's Account", entities.AccountTypePersonal)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}
	_ = accounts.Save(ctx, personalAccount)
	_ = accounts.SaveMember(ctx, "account-existing", "agent-existing", entities.RoleOwner)

	provider := NewMockOAuthProvider("mock-idp")
	providers := application.OAuthProviderRegistry{"mock-idp": provider}

	svc := application.NewDefaultAuthenticationService(
		providers, agents, credentials, sessions, accounts,
	)

	userInfo := application.UserInfo{
		ProviderUserID: "provider-user-42",
		Email:          "alice@example.com",
		DisplayName:    "Alice",
		Provider:       "mock-idp",
	}

	foundAgent, foundCred, foundAccount, err := svc.FindOrCreateAgent(ctx, userInfo)
	if err != nil {
		t.Fatalf("FindOrCreateAgent() error: %v", err)
	}

	if foundAgent.GetID() != "agent-existing" {
		t.Errorf("agent ID = %q, want %q", foundAgent.GetID(), "agent-existing")
	}
	if foundCred.GetID() != "cred-existing" {
		t.Errorf("credential ID = %q, want %q", foundCred.GetID(), "cred-existing")
	}
	if foundAccount == nil {
		t.Fatal("expected non-nil account")
	}
	if foundAccount.GetID() != "account-existing" {
		t.Errorf("account ID = %q, want %q", foundAccount.GetID(), "account-existing")
	}
}

func TestAuthenticationFlow_InvalidProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	provider := NewMockOAuthProvider("mock-idp")
	providers := application.OAuthProviderRegistry{"mock-idp": provider}

	svc := application.NewDefaultAuthenticationService(
		providers,
		NewMemoryAgentRepository(),
		NewMemoryCredentialRepository(),
		NewMemorySessionRepository(),
		NewMemoryAccountRepository(),
	)

	_, err := svc.InitiateAuthFlow(ctx, "unknown-provider", "https://app.example.com/callback")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !errors.Is(err, application.ErrInvalidProvider) {
		t.Errorf("error = %v, want ErrInvalidProvider", err)
	}
}

func TestAuthenticationFlow_SessionExpiry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	provider := NewMockOAuthProvider("mock-idp")
	providers := application.OAuthProviderRegistry{"mock-idp": provider}

	svc := application.NewDefaultAuthenticationService(
		providers,
		NewMemoryAgentRepository(),
		NewMemoryCredentialRepository(),
		NewMemorySessionRepository(),
		NewMemoryAccountRepository(),
	)

	// Create a session that is already expired (zero duration)
	session, err := svc.CreateSession(ctx, "agent-1", "cred-1", "127.0.0.1", "Test/1.0", 0)
	if err != nil {
		t.Fatalf("CreateSession() error: %v", err)
	}
	session.ClearUncommittedEvents()

	// Wait a tiny bit to ensure the session is past its expiry
	time.Sleep(time.Millisecond)

	_, err = svc.ValidateSession(ctx, session.GetID())
	if err == nil {
		t.Fatal("expected error for expired session")
	}
	if !errors.Is(err, application.ErrSessionExpired) {
		t.Errorf("error = %v, want ErrSessionExpired", err)
	}
}

func TestAuthenticationFlow_IdentityContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	identity := &auth.Identity{
		AgentID:         "agent-ctx-test",
		AccountIDs:      []string{"account-1", "account-2"},
		ActiveAccountID: "account-1",
	}

	authedCtx := auth.ContextWithAgent(ctx, identity)
	recovered := auth.AgentFromCtx(authedCtx)

	if recovered == nil {
		t.Fatal("expected non-nil identity from context")
	}
	if recovered.AgentID != "agent-ctx-test" {
		t.Errorf("AgentID = %q, want %q", recovered.AgentID, "agent-ctx-test")
	}
	if recovered.ActiveAccountID != "account-1" {
		t.Errorf("ActiveAccountID = %q, want %q", recovered.ActiveAccountID, "account-1")
	}
	if len(recovered.AccountIDs) != 2 {
		t.Errorf("len(AccountIDs) = %d, want 2", len(recovered.AccountIDs))
	}

	// No identity in bare context
	noIdentity := auth.AgentFromCtx(ctx)
	if noIdentity != nil {
		t.Error("expected nil identity from bare context")
	}
}

func TestAuthenticationFlow_ResourceOwnership(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	identity := &auth.Identity{
		AgentID:         "agent-owner",
		AccountIDs:      []string{"account-owned"},
		ActiveAccountID: "account-owned",
	}
	authedCtx := auth.ContextWithAgent(ctx, identity)

	// ResourceOwnershipFromCtx
	ownership, err := auth.ResourceOwnershipFromCtx(authedCtx)
	if err != nil {
		t.Fatalf("ResourceOwnershipFromCtx() error: %v", err)
	}
	if ownership.AccountID != "account-owned" {
		t.Errorf("AccountID = %q, want %q", ownership.AccountID, "account-owned")
	}
	if ownership.CreatedByAgentID != "agent-owner" {
		t.Errorf("CreatedByAgentID = %q, want %q", ownership.CreatedByAgentID, "agent-owner")
	}

	// VerifyAccountAccess — matching account
	if err := auth.VerifyAccountAccess(authedCtx, "account-owned"); err != nil {
		t.Fatalf("VerifyAccountAccess() for same account: %v", err)
	}

	// VerifyAccountAccess — mismatched account
	err = auth.VerifyAccountAccess(authedCtx, "account-other")
	if !errors.Is(err, auth.ErrAccountMismatch) {
		t.Errorf("error = %v, want ErrAccountMismatch", err)
	}

	// No identity in context
	_, err = auth.ResourceOwnershipFromCtx(ctx)
	if !errors.Is(err, auth.ErrNoIdentity) {
		t.Errorf("error = %v, want ErrNoIdentity", err)
	}

	err = auth.VerifyAccountAccess(ctx, "any-account")
	if !errors.Is(err, auth.ErrNoIdentity) {
		t.Errorf("error = %v, want ErrNoIdentity", err)
	}
}
