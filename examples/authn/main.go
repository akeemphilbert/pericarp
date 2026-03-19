// Package main demonstrates the Pericarp authentication lifecycle.
//
// Run with:
//
//	go run ./examples/authn/
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	authjwt "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/jwt"
	esInfra "github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
)

// FlowResult captures the outputs of RunAuthenticationFlow for testing.
type FlowResult struct {
	AuthRequest  *application.AuthRequest
	AuthResult   *application.AuthResult
	AgentID      string
	CredentialID string
	AccountID    string
	SessionID    string
	SessionInfo  *application.SessionInfo
	Token        string
	Claims       *application.PericarpClaims
}

// RunAuthenticationFlow runs the full authentication lifecycle and returns
// the result for inspection by tests.
func RunAuthenticationFlow(ctx context.Context) (*FlowResult, error) {
	result := &FlowResult{}

	// --- 1. Generate RSA key at runtime ---
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}
	fmt.Println("[1] RSA key generated")

	// --- 2. Create RSAJWTService ---
	jwtSvc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(rsaKey))
	fmt.Println("[2] RSAJWTService created")

	// --- 3. Wire DefaultAuthenticationService ---
	provider := NewMockOAuthProvider("mock-idp")
	providers := application.OAuthProviderRegistry{
		"mock-idp": provider,
	}
	agents := NewMemoryAgentRepository()
	credentials := NewMemoryCredentialRepository()
	sessions := NewMemorySessionRepository()
	accounts := NewMemoryAccountRepository()
	eventStore := esInfra.NewMemoryStore()

	svc := application.NewDefaultAuthenticationService(
		providers, agents, credentials, sessions, accounts,
		application.WithEventStore(eventStore),
		application.WithJWTService(jwtSvc),
	)
	fmt.Println("[3] DefaultAuthenticationService wired")

	// --- 4. InitiateAuthFlow ---
	authReq, err := svc.InitiateAuthFlow(ctx, "mock-idp", "https://app.example.com/callback")
	if err != nil {
		return nil, fmt.Errorf("InitiateAuthFlow: %w", err)
	}
	result.AuthRequest = authReq
	fmt.Printf("[4] AuthFlow initiated: URL=%s, state=%s\n", authReq.AuthURL, authReq.State[:8]+"...")

	// --- 5. ExchangeCode ---
	authResult, err := svc.ExchangeCode(ctx, "auth-code-abc", authReq.CodeVerifier, "mock-idp", "https://app.example.com/callback")
	if err != nil {
		return nil, fmt.Errorf("ExchangeCode: %w", err)
	}
	result.AuthResult = authResult
	fmt.Printf("[5] Code exchanged: AccessToken=%s, TokenType=%s\n", authResult.AccessToken[:16]+"...", authResult.TokenType)

	// --- 6. ValidateState ---
	if err := svc.ValidateState(ctx, authReq.State, authReq.State); err != nil {
		return nil, fmt.Errorf("ValidateState: %w", err)
	}
	fmt.Println("[6] State validated (constant-time comparison)")

	// --- 7. FindOrCreateAgent ---
	userInfo := authResult.UserInfo
	agent, credential, account, err := svc.FindOrCreateAgent(ctx, userInfo)
	if err != nil {
		return nil, fmt.Errorf("FindOrCreateAgent: %w", err)
	}
	result.AgentID = agent.GetID()
	result.CredentialID = credential.GetID()
	result.AccountID = account.GetID()
	fmt.Printf("[7] Agent created: ID=%s, Name=%s, AccountID=%s\n", agent.GetID(), agent.Name(), account.GetID())

	// --- 8. CreateSession ---
	session, err := svc.CreateSession(ctx, agent.GetID(), credential.GetID(), "192.168.1.1", "ExampleApp/1.0", 24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("CreateSession: %w", err)
	}
	session.ClearUncommittedEvents()
	result.SessionID = session.GetID()
	fmt.Printf("[8] Session created: ID=%s, ExpiresAt=%s\n", session.GetID(), session.ExpiresAt().Format(time.RFC3339))

	// --- 9. ValidateSession ---
	sessionInfo, err := svc.ValidateSession(ctx, session.GetID())
	if err != nil {
		return nil, fmt.Errorf("ValidateSession: %w", err)
	}
	result.SessionInfo = sessionInfo
	fmt.Printf("[9] Session valid: AgentID=%s, ExpiresAt=%s\n", sessionInfo.AgentID, sessionInfo.ExpiresAt.Format(time.RFC3339))

	// --- 10. IssueIdentityToken + ValidateToken ---
	token, err := svc.IssueIdentityToken(ctx, agent, account.GetID())
	if err != nil {
		return nil, fmt.Errorf("IssueIdentityToken: %w", err)
	}
	result.Token = token
	fmt.Printf("[10] JWT issued: %s...\n", token[:32])

	claims, err := jwtSvc.ValidateToken(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("ValidateToken: %w", err)
	}
	result.Claims = claims
	fmt.Printf("     JWT validated: AgentID=%s, ActiveAccount=%s\n", claims.AgentID, claims.ActiveAccountID)

	// --- 11. ContextWithAgent / AgentFromCtx ---
	identity := &auth.Identity{
		AgentID:         agent.GetID(),
		AccountIDs:      []string{account.GetID()},
		ActiveAccountID: account.GetID(),
	}
	authedCtx := auth.ContextWithAgent(ctx, identity)
	recovered := auth.AgentFromCtx(authedCtx)
	fmt.Printf("[11] Identity context: AgentID=%s, ActiveAccount=%s\n", recovered.AgentID, recovered.ActiveAccountID)

	// --- 12. ResourceOwnershipFromCtx / VerifyAccountAccess ---
	ownership, err := auth.ResourceOwnershipFromCtx(authedCtx)
	if err != nil {
		return nil, fmt.Errorf("ResourceOwnershipFromCtx: %w", err)
	}
	fmt.Printf("[12] ResourceOwnership: AccountID=%s, CreatedBy=%s\n", ownership.AccountID, ownership.CreatedByAgentID)

	if err := auth.VerifyAccountAccess(authedCtx, account.GetID()); err != nil {
		return nil, fmt.Errorf("VerifyAccountAccess (same account): %w", err)
	}
	fmt.Println("     VerifyAccountAccess (same account): OK")

	mismatchErr := auth.VerifyAccountAccess(authedCtx, "other-account-id")
	if !errors.Is(mismatchErr, auth.ErrAccountMismatch) {
		return nil, fmt.Errorf("expected ErrAccountMismatch, got: %v", mismatchErr)
	}
	fmt.Println("     VerifyAccountAccess (different account): ErrAccountMismatch (correct)")

	// --- 13. RevokeSession ---
	if err := svc.RevokeSession(ctx, session.GetID()); err != nil {
		return nil, fmt.Errorf("RevokeSession: %w", err)
	}
	fmt.Printf("[13] Session revoked: ID=%s\n", session.GetID())

	_, revokedErr := svc.ValidateSession(ctx, session.GetID())
	if !errors.Is(revokedErr, application.ErrSessionRevoked) {
		return nil, fmt.Errorf("expected ErrSessionRevoked, got: %v", revokedErr)
	}
	fmt.Println("     ValidateSession after revoke: ErrSessionRevoked (correct)")

	return result, nil
}

func main() {
	ctx := context.Background()
	fmt.Println("=== Pericarp Authentication Lifecycle Demo ===")
	fmt.Println()

	result, err := RunAuthenticationFlow(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("=== All steps completed successfully ===")
	fmt.Printf("Agent: %s | Account: %s | Session: %s\n", result.AgentID, result.AccountID, result.SessionID)
}
