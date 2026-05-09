package jwt_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth"
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

	tokenString, err := svc.IssueToken(context.Background(), agent, accounts, "acc-1", nil, nil)
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

	_, err := svc.IssueToken(context.Background(), agent, nil, "", nil, nil)
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

	_, err := svc.IssueToken(ctx, agent, nil, "", nil, nil)
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

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "", nil, nil)
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
	tokenString, err := svcA.IssueToken(context.Background(), agent, nil, "", nil, nil)
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

	tokenString, err := svc.IssueToken(context.Background(), agent, []*entities.Account{}, "", nil, nil)
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

	tokenString, err := svc.IssueToken(context.Background(), agent, accounts, "acc-2", nil, nil)
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

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "", nil, nil)
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

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "", nil, nil)
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

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "", nil, nil)
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

	_, err := svc.IssueToken(context.Background(), nil, nil, "", nil, nil)
	if err == nil {
		t.Fatal("expected error for nil agent, got nil")
	}
}

func TestIssueToken_SubscriptionRoundTrip(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	agent := createTestAgent(t, "agent-1", "Test User")
	expires := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)
	subscription := &auth.SubscriptionClaim{
		Status:    auth.SubscriptionStatusActive,
		Plan:      "pro",
		Provider:  "stripe",
		ExpiresAt: expires,
		Metadata:  map[string]any{"price_id": "price_123"},
	}

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "", subscription, nil)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	claims, err := svc.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.Subscription == nil {
		t.Fatal("expected subscription claim, got nil")
	}
	if claims.Subscription.Status != auth.SubscriptionStatusActive {
		t.Errorf("Status = %q, want %q", claims.Subscription.Status, auth.SubscriptionStatusActive)
	}
	if claims.Subscription.Plan != "pro" {
		t.Errorf("Plan = %q, want %q", claims.Subscription.Plan, "pro")
	}
	if claims.Subscription.Provider != "stripe" {
		t.Errorf("Provider = %q, want %q", claims.Subscription.Provider, "stripe")
	}
	if !claims.Subscription.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt = %v, want %v", claims.Subscription.ExpiresAt, expires)
	}
	if got := claims.Subscription.Metadata["price_id"]; got != "price_123" {
		t.Errorf("Metadata[price_id] = %v, want %q", got, "price_123")
	}
	if !claims.Subscription.IsActive() {
		t.Error("IsActive() = false, want true for active status")
	}
}

func TestIssueToken_NoSubscription_OmitsClaim(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	agent := createTestAgent(t, "agent-1", "Test User")

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "", nil, nil)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	claims, err := svc.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.Subscription != nil {
		t.Errorf("expected nil subscription, got %+v", claims.Subscription)
	}
}

func TestReissueToken_PreservesSubscription(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))

	agent := createTestAgent(t, "agent-1", "Test User")
	subscription := &auth.SubscriptionClaim{Status: auth.SubscriptionStatusTrialing, Plan: "trial"}

	originalToken, err := svc.IssueToken(context.Background(), agent, nil, "acc-1", subscription, nil)
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
	if newClaims.Subscription == nil {
		t.Fatal("reissued token should preserve subscription claim")
	}
	if newClaims.Subscription.Status != auth.SubscriptionStatusTrialing {
		t.Errorf("Status = %q, want %q", newClaims.Subscription.Status, auth.SubscriptionStatusTrialing)
	}
	if newClaims.Subscription.Plan != "trial" {
		t.Errorf("Plan = %q, want %q", newClaims.Subscription.Plan, "trial")
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

	originalToken, err := svc.IssueToken(context.Background(), agent, accounts, "acc-1", nil, nil)
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

	originalToken, err := svc.IssueToken(context.Background(), agent, accounts, "acc-1", nil, nil)
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

// --- Extras / custom claim tests ---

func TestIssueToken_ExtrasRoundTrip(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	agent := createTestAgent(t, "agent-1", "Test User")

	extras := map[string]any{
		"role":      "admin",
		"tenant_id": "tenant-42",
		"flags":     []any{"beta", "experimental"},
		"limits":    map[string]any{"max_seats": float64(10)},
	}

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "", nil, extras)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	claims, err := svc.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if got := claims.Extras["role"]; got != "admin" {
		t.Errorf("Extras[role] = %v, want %q", got, "admin")
	}
	if got := claims.Extras["tenant_id"]; got != "tenant-42" {
		t.Errorf("Extras[tenant_id] = %v, want %q", got, "tenant-42")
	}
	flags, ok := claims.Extras["flags"].([]any)
	if !ok {
		t.Fatalf("Extras[flags] is %T, want []any", claims.Extras["flags"])
	}
	if len(flags) != 2 || flags[0] != "beta" || flags[1] != "experimental" {
		t.Errorf("Extras[flags] = %v, want [beta experimental]", flags)
	}
	limits, ok := claims.Extras["limits"].(map[string]any)
	if !ok {
		t.Fatalf("Extras[limits] is %T, want map[string]any", claims.Extras["limits"])
	}
	if limits["max_seats"] != float64(10) {
		t.Errorf("Extras[limits][max_seats] = %v, want 10", limits["max_seats"])
	}
}

func TestIssueToken_ReservedClaimRejected(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	agent := createTestAgent(t, "agent-1", "Test User")

	for _, name := range application.ReservedClaimNames() {
		t.Run(name, func(t *testing.T) {
			extras := map[string]any{name: "attacker-value"}
			_, err := svc.IssueToken(context.Background(), agent, nil, "", nil, extras)
			if !errors.Is(err, application.ErrReservedClaim) {
				t.Fatalf("expected ErrReservedClaim for key %q, got %v", name, err)
			}
		})
	}
}

// decodePayload returns the parsed JSON payload of a JWT (the middle
// base64url segment). Used by tests that need to assert exactly which
// top-level keys reach the wire — Extras=nil/empty must not produce
// stray "extras":null entries that would confuse downstream consumers.
func decodePayload(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d segments, want 3", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("base64-decode payload: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return out
}

func TestIssueToken_NilExtras_NoExtrasKey(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	agent := createTestAgent(t, "agent-1", "Test User")

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "", nil, nil)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}
	claims, err := svc.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.Extras != nil {
		t.Errorf("Extras = %v, want nil for nil input", claims.Extras)
	}
	payload := decodePayload(t, tokenString)
	if _, has := payload["extras"]; has {
		t.Errorf("payload contains stray %q key: %v", "extras", payload)
	}
}

func TestIssueToken_EmptyExtras_NoExtrasKey(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	agent := createTestAgent(t, "agent-1", "Test User")

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "", nil, map[string]any{})
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}
	claims, err := svc.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.Extras != nil {
		t.Errorf("Extras = %v, want nil for empty input", claims.Extras)
	}
	payload := decodePayload(t, tokenString)
	if _, has := payload["extras"]; has {
		t.Errorf("payload contains stray %q key: %v", "extras", payload)
	}
}

func TestReissueToken_PreservesExtras(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	agent := createTestAgent(t, "agent-1", "Test User")

	extras := map[string]any{"role": "owner", "tier": "gold"}
	originalToken, err := svc.IssueToken(context.Background(), agent, nil, "acc-1", nil, extras)
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
	if newClaims.Extras["role"] != "owner" {
		t.Errorf("reissued Extras[role] = %v, want %q", newClaims.Extras["role"], "owner")
	}
	if newClaims.Extras["tier"] != "gold" {
		t.Errorf("reissued Extras[tier] = %v, want %q", newClaims.Extras["tier"], "gold")
	}
}

// TestPericarpClaims_MarshalRejectsReservedExtras exercises the
// defense-in-depth backstop: if a caller constructs PericarpClaims
// directly with reserved keys in Extras (bypassing ValidateExtras at
// IssueToken time), MarshalJSON must refuse rather than silently drop
// the offending keys — a refused token is loud, a silently-stripped
// claim surfaces as a confusing authorization failure far downstream.
func TestPericarpClaims_MarshalRejectsReservedExtras(t *testing.T) {
	t.Parallel()

	c := application.PericarpClaims{
		AgentID: "agent-real",
		Extras: map[string]any{
			"agent_id": "agent-spoof",
			"role":     "admin",
		},
	}

	_, err := c.MarshalJSON()
	if !errors.Is(err, application.ErrReservedClaim) {
		t.Fatalf("MarshalJSON err = %v, want ErrReservedClaim", err)
	}
}

// TestPericarpClaims_UnmarshalDropsReservedSiblings is the parse-side
// twin of the marshal backstop: an externally minted token that
// presents reserved keys as siblings of the core fields (e.g., an
// attacker stuffing "agent_id" into the payload trying to land it in
// Extras for a downstream authz check that reads the map) must not
// surface in claims.Extras. UnmarshalJSON silently excludes them
// because rejecting the whole token would let a malicious issuer DOS
// validation; surfacing them in Extras would expand the attack surface.
func TestPericarpClaims_UnmarshalDropsReservedSiblings(t *testing.T) {
	t.Parallel()

	payload := []byte(`{
		"sub": "agent-real",
		"agent_id": "agent-real",
		"account_ids": ["acc-1"],
		"active_account_id": "acc-1",
		"role": "admin",
		"tenant_id": "t-42"
	}`)

	var c application.PericarpClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if c.AgentID != "agent-real" {
		t.Errorf("AgentID = %q, want %q", c.AgentID, "agent-real")
	}
	for _, name := range application.ReservedClaimNames() {
		if _, has := c.Extras[name]; has {
			t.Errorf("Extras[%q] is set; reserved keys must be excluded from Extras", name)
		}
	}
	if c.Extras["role"] != "admin" {
		t.Errorf("Extras[role] = %v, want %q", c.Extras["role"], "admin")
	}
	if c.Extras["tenant_id"] != "t-42" {
		t.Errorf("Extras[tenant_id] = %v, want %q", c.Extras["tenant_id"], "t-42")
	}
}

func TestIssueToken_SubscriptionAndExtrasCoexist(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	agent := createTestAgent(t, "agent-1", "Test User")

	subscription := &auth.SubscriptionClaim{
		Status:   auth.SubscriptionStatusActive,
		Plan:     "pro",
		Provider: "stripe",
	}
	extras := map[string]any{"role": "admin", "tenant_id": "t-42"}

	tokenString, err := svc.IssueToken(context.Background(), agent, nil, "", subscription, extras)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}
	claims, err := svc.ValidateToken(context.Background(), tokenString)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims.Subscription == nil || claims.Subscription.Plan != "pro" {
		t.Errorf("Subscription = %+v, want plan=pro", claims.Subscription)
	}
	if claims.Extras["role"] != "admin" {
		t.Errorf("Extras[role] = %v, want admin", claims.Extras["role"])
	}
	if claims.Extras["tenant_id"] != "t-42" {
		t.Errorf("Extras[tenant_id] = %v, want t-42", claims.Extras["tenant_id"])
	}
}

func TestReissueToken_RejectsMutatedReservedExtras(t *testing.T) {
	t.Parallel()

	key := generateTestKey(t)
	svc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(key))
	agent := createTestAgent(t, "agent-1", "Test User")

	originalToken, err := svc.IssueToken(context.Background(), agent, nil, "acc-1", nil, map[string]any{"role": "user"})
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}
	claims, err := svc.ValidateToken(context.Background(), originalToken)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	// Simulate in-memory tampering between Validate and Reissue, or a
	// future reserved-set expansion catching a once-valid extras key.
	claims.Extras["sub"] = "agent-spoof"

	_, err = svc.ReissueToken(context.Background(), claims, "acc-2")
	if !errors.Is(err, application.ErrReservedClaim) {
		t.Fatalf("ReissueToken err = %v, want ErrReservedClaim", err)
	}
}

func TestValidateExtras(t *testing.T) {
	t.Parallel()

	if err := application.ValidateExtras(nil); err != nil {
		t.Errorf("nil extras: %v, want nil", err)
	}
	if err := application.ValidateExtras(map[string]any{}); err != nil {
		t.Errorf("empty extras: %v, want nil", err)
	}
	if err := application.ValidateExtras(map[string]any{"role": "admin", "tenant": "t1"}); err != nil {
		t.Errorf("non-reserved extras: %v, want nil", err)
	}
}

func TestValidateExtras_MultipleReservedReportedDeterministically(t *testing.T) {
	t.Parallel()

	err := application.ValidateExtras(map[string]any{
		"exp":      1,
		"iss":      "x",
		"agent_id": "spoof",
		"role":     "admin",
	})
	if !errors.Is(err, application.ErrReservedClaim) {
		t.Fatalf("err = %v, want ErrReservedClaim", err)
	}
	msg := err.Error()
	for _, name := range []string{"agent_id", "exp", "iss"} {
		if !strings.Contains(msg, name) {
			t.Errorf("error message missing reserved key %q: %s", name, msg)
		}
	}
	if strings.Contains(msg, "role") {
		t.Errorf("error message references non-reserved key %q: %s", "role", msg)
	}
	// Sorted output: agent_id < exp < iss alphabetically.
	if i, j, k := strings.Index(msg, "agent_id"), strings.Index(msg, "exp"), strings.Index(msg, "iss"); !(i < j && j < k) {
		t.Errorf("reserved keys not sorted in message %q (positions: agent_id=%d, exp=%d, iss=%d)", msg, i, j, k)
	}
}

func TestIsReservedClaim(t *testing.T) {
	t.Parallel()

	for _, name := range application.ReservedClaimNames() {
		if !application.IsReservedClaim(name) {
			t.Errorf("IsReservedClaim(%q) = false, want true (in ReservedClaimNames)", name)
		}
	}
	for _, name := range []string{"role", "tenant_id", "", "extras", "permissions"} {
		if application.IsReservedClaim(name) {
			t.Errorf("IsReservedClaim(%q) = true, want false", name)
		}
	}
}
