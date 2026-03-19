package jwt_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	authjwt "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/jwt"
)

func generateTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}
	return key
}

func createTestAgent(t *testing.T, id, name string) *entities.Agent {
	t.Helper()
	agent, err := new(entities.Agent).With(id, name, entities.AgentTypePerson)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	return agent
}

func createTestAccount(t *testing.T, id, name string) *entities.Account {
	t.Helper()
	account, err := new(entities.Account).With(id, name, entities.AccountTypePersonal)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}
	return account
}

func TestIssueAndValidate_RoundTrip(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	agent := createTestAgent(t, "agent-1", "Test User")
	accounts := []*entities.Account{
		createTestAccount(t, "acc-1", "Account One"),
		createTestAccount(t, "acc-2", "Account Two"),
	}

	tokenString, err := svc.IssueToken(context.Background(), agent, accounts, "acc-1")
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	claims, err := svc.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", claims.AgentID, "agent-1")
	}
	if claims.ActiveAccountID != "acc-1" {
		t.Errorf("ActiveAccountID = %q, want %q", claims.ActiveAccountID, "acc-1")
	}
	if len(claims.AccountIDs) != 2 {
		t.Fatalf("AccountIDs length = %d, want 2", len(claims.AccountIDs))
	}
	if claims.AccountIDs[0] != "acc-1" {
		t.Errorf("AccountIDs[0] = %q, want %q", claims.AccountIDs[0], "acc-1")
	}
	if claims.AccountIDs[1] != "acc-2" {
		t.Errorf("AccountIDs[1] = %q, want %q", claims.AccountIDs[1], "acc-2")
	}
	if claims.Issuer != "pericarp" {
		t.Errorf("Issuer = %q, want %q", claims.Issuer, "pericarp")
	}
	if claims.Subject != "agent-1" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "agent-1")
	}
	if claims.ID == "" {
		t.Error("JWT ID should not be empty")
	}
}

func TestIssueToken_NoSigningKey(t *testing.T) {
	t.Parallel()

	svc := authjwt.NewRSAJWTService() // no key
	agent := createTestAgent(t, "agent-1", "Test User")

	_, err := svc.IssueToken(context.Background(), agent, nil, "")
	if err != application.ErrNoSigningKey {
		t.Errorf("expected ErrNoSigningKey, got %v", err)
	}
}

func TestIssueToken_CancelledContext(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	agent := createTestAgent(t, "agent-1", "Test User")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.IssueToken(ctx, agent, nil, "")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(
		authjwt.WithSigningKey(key),
		authjwt.WithTokenTTL(1*time.Millisecond),
	)
	agent := createTestAgent(t, "agent-1", "Test User")

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "")
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	// Wait for expiration
	time.Sleep(10 * time.Millisecond)

	_, err = svc.ValidateToken(context.Background(), tokenString)
	if err != application.ErrTokenExpired {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestValidateToken_WrongKey(t *testing.T) {
	t.Parallel()

	keyA := generateTestKey(t)
	keyB := generateTestKey(t)

	svcA := authjwt.NewRSAJWTService(authjwt.WithSigningKey(keyA))
	svcB := authjwt.NewRSAJWTService(authjwt.WithSigningKey(keyB))

	agent := createTestAgent(t, "agent-1", "Test User")
	tokenString, err := svcA.IssueToken(context.Background(), agent, nil, "")
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	_, err = svcB.ValidateToken(context.Background(), tokenString)
	if !errors.Is(err, application.ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestValidateToken_MalformedToken(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	_, err := svc.ValidateToken(context.Background(), "not-a-jwt")
	if !errors.Is(err, application.ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestIssueToken_EmptyAccounts(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	agent := createTestAgent(t, "agent-1", "Test User")

	tokenString, err := svc.IssueToken(context.Background(), agent, []*entities.Account{}, "")
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	claims, err := svc.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if len(claims.AccountIDs) != 0 {
		t.Errorf("AccountIDs length = %d, want 0", len(claims.AccountIDs))
	}
	if claims.ActiveAccountID != "" {
		t.Errorf("ActiveAccountID = %q, want empty", claims.ActiveAccountID)
	}
}

func TestIssueToken_MultipleAccounts(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	agent := createTestAgent(t, "agent-1", "Test User")
	accounts := []*entities.Account{
		createTestAccount(t, "acc-1", "Account One"),
		createTestAccount(t, "acc-2", "Account Two"),
		createTestAccount(t, "acc-3", "Account Three"),
	}

	tokenString, err := svc.IssueToken(context.Background(), agent, accounts, "acc-2")
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	claims, err := svc.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if len(claims.AccountIDs) != 3 {
		t.Fatalf("AccountIDs length = %d, want 3", len(claims.AccountIDs))
	}
	for i, expected := range []string{"acc-1", "acc-2", "acc-3"} {
		if claims.AccountIDs[i] != expected {
			t.Errorf("AccountIDs[%d] = %q, want %q", i, claims.AccountIDs[i], expected)
		}
	}
}

func TestSubjectEqualsAgentID(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	agent := createTestAgent(t, "agent-42", "Test User")

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "")
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	claims, err := svc.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.Subject != claims.AgentID {
		t.Errorf("Subject %q != AgentID %q", claims.Subject, claims.AgentID)
	}
}

func TestCustomTTLAndIssuer(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(
		authjwt.WithSigningKey(key),
		authjwt.WithTokenTTL(1*time.Hour),
		authjwt.WithIssuer("my-service"),
	)
	agent := createTestAgent(t, "agent-1", "Test User")

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "")
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	claims, err := svc.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.Issuer != "my-service" {
		t.Errorf("Issuer = %q, want %q", claims.Issuer, "my-service")
	}

	// Verify TTL: ExpiresAt should be roughly 1 hour from IssuedAt
	ttl := claims.ExpiresAt.Sub(claims.IssuedAt.Time)
	if ttl < 59*time.Minute || ttl > 61*time.Minute {
		t.Errorf("TTL = %v, want ~1h", ttl)
	}
}

func TestValidateToken_CancelledContext(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	agent := createTestAgent(t, "agent-1", "Test User")

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "")
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = svc.ValidateToken(ctx, tokenString)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestValidateToken_NoPublicKey(t *testing.T) {
	t.Parallel()

	svc := authjwt.NewRSAJWTService() // no key

	_, err := svc.ValidateToken(context.Background(), "some-token")
	if err != application.ErrNoSigningKey {
		t.Errorf("expected ErrNoSigningKey, got %v", err)
	}
}

func TestIssueToken_NilAgent(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	_, err := svc.IssueToken(context.Background(), nil, nil, "")
	if err == nil {
		t.Fatal("expected error for nil agent, got nil")
	}
}

// --- Invite Token Tests ---

func TestInviteToken_RoundTrip(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	tokenString, err := svc.IssueInviteToken(context.Background(), "invite-123", 7*24*time.Hour)
	if err != nil {
		t.Fatalf("IssueInviteToken failed: %v", err)
	}

	claims, err := svc.ValidateInviteToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateInviteToken failed: %v", err)
	}

	if claims.InviteID != "invite-123" {
		t.Errorf("InviteID = %q, want %q", claims.InviteID, "invite-123")
	}
	if claims.Subject != "invite-123" {
		t.Errorf("Subject = %q, want %q", claims.Subject, "invite-123")
	}
	if claims.Issuer != "pericarp" {
		t.Errorf("Issuer = %q, want %q", claims.Issuer, "pericarp")
	}
	if claims.ID == "" {
		t.Error("JWT ID should not be empty")
	}
}

func TestInviteToken_Expired(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	tokenString, err := svc.IssueInviteToken(context.Background(), "invite-123", 1*time.Millisecond)
	if err != nil {
		t.Fatalf("IssueInviteToken failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	_, err = svc.ValidateInviteToken(context.Background(), tokenString)
	if err != application.ErrTokenExpired {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestInviteToken_WrongKey(t *testing.T) {
	t.Parallel()

	keyA := generateTestKey(t)
	keyB := generateTestKey(t)

	svcA := authjwt.NewRSAJWTService(authjwt.WithSigningKey(keyA))
	svcB := authjwt.NewRSAJWTService(authjwt.WithSigningKey(keyB))

	tokenString, err := svcA.IssueInviteToken(context.Background(), "invite-123", 1*time.Hour)
	if err != nil {
		t.Fatalf("IssueInviteToken failed: %v", err)
	}

	_, err = svcB.ValidateInviteToken(context.Background(), tokenString)
	if !errors.Is(err, application.ErrTokenInvalid) {
		t.Errorf("expected ErrTokenInvalid, got %v", err)
	}
}

func TestInviteToken_NoSigningKey(t *testing.T) {
	t.Parallel()

	svc := authjwt.NewRSAJWTService() // no key

	_, err := svc.IssueInviteToken(context.Background(), "invite-123", 1*time.Hour)
	if err != application.ErrNoSigningKey {
		t.Errorf("expected ErrNoSigningKey, got %v", err)
	}
}

func TestInviteToken_EmptyInviteID(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	_, err := svc.IssueInviteToken(context.Background(), "", 1*time.Hour)
	if err == nil {
		t.Fatal("expected error for empty invite ID, got nil")
	}
}

// --- ReissueToken Tests ---

func TestReissueToken_RoundTrip(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	agent := createTestAgent(t, "agent-1", "Test User")
	accounts := []*entities.Account{
		createTestAccount(t, "acc-1", "Account One"),
		createTestAccount(t, "acc-2", "Account Two"),
	}

	originalToken, err := svc.IssueToken(context.Background(), agent, accounts, "acc-1")
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	originalClaims, err := svc.ValidateToken(context.Background(), originalToken)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	reissuedToken, err := svc.ReissueToken(context.Background(), originalClaims, "acc-2")
	if err != nil {
		t.Fatalf("ReissueToken failed: %v", err)
	}

	newClaims, err := svc.ValidateToken(context.Background(), reissuedToken)
	if err != nil {
		t.Fatalf("ValidateToken on reissued token failed: %v", err)
	}

	if newClaims.ActiveAccountID != "acc-2" {
		t.Errorf("ActiveAccountID = %q, want %q", newClaims.ActiveAccountID, "acc-2")
	}
	if newClaims.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", newClaims.AgentID, "agent-1")
	}
	if len(newClaims.AccountIDs) != 2 {
		t.Fatalf("AccountIDs length = %d, want 2", len(newClaims.AccountIDs))
	}
	if newClaims.AccountIDs[0] != "acc-1" || newClaims.AccountIDs[1] != "acc-2" {
		t.Errorf("AccountIDs = %v, want [acc-1, acc-2]", newClaims.AccountIDs)
	}
	if newClaims.Subject != "agent-1" {
		t.Errorf("Subject = %q, want %q", newClaims.Subject, "agent-1")
	}
}

func TestReissueToken_FreshTimestamps(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	agent := createTestAgent(t, "agent-1", "Test User")
	accounts := []*entities.Account{
		createTestAccount(t, "acc-1", "Account One"),
	}

	originalToken, err := svc.IssueToken(context.Background(), agent, accounts, "acc-1")
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	originalClaims, err := svc.ValidateToken(context.Background(), originalToken)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	// Sleep just over 1 second so JWT second-precision timestamps differ.
	time.Sleep(1100 * time.Millisecond)

	reissuedToken, err := svc.ReissueToken(context.Background(), originalClaims, "acc-1")
	if err != nil {
		t.Fatalf("ReissueToken failed: %v", err)
	}

	newClaims, err := svc.ValidateToken(context.Background(), reissuedToken)
	if err != nil {
		t.Fatalf("ValidateToken on reissued token failed: %v", err)
	}

	if newClaims.ID == originalClaims.ID {
		t.Error("reissued token should have a different JWT ID")
	}
	if !newClaims.IssuedAt.After(originalClaims.IssuedAt.Time) {
		t.Error("reissued token IssuedAt should be after original")
	}
	if !newClaims.ExpiresAt.After(originalClaims.ExpiresAt.Time) {
		t.Error("reissued token ExpiresAt should be after original")
	}
}

func TestReissueToken_NilClaims(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	_, err := svc.ReissueToken(context.Background(), nil, "acc-1")
	if err == nil {
		t.Fatal("expected error for nil claims, got nil")
	}
}

func TestReissueToken_NoSigningKey(t *testing.T) {
	t.Parallel()

	svc := authjwt.NewRSAJWTService() // no key

	claims := &application.PericarpClaims{AgentID: "agent-1"}
	_, err := svc.ReissueToken(context.Background(), claims, "acc-1")
	if err != application.ErrNoSigningKey {
		t.Errorf("expected ErrNoSigningKey, got %v", err)
	}
}

func TestReissueToken_CancelledContext(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	claims := &application.PericarpClaims{AgentID: "agent-1"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.ReissueToken(ctx, claims, "acc-1")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
