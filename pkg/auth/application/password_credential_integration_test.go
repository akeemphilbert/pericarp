package application_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"strings"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	gorminfra "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/database/gorm"
	authjwt "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/jwt"
	esinfra "github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func newSQLiteAuthService(t *testing.T) *application.DefaultAuthenticationService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := gorminfra.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{},
		gorminfra.NewAgentRepository(db),
		gorminfra.NewCredentialRepository(db),
		gorminfra.NewAuthSessionRepository(db),
		gorminfra.NewAccountRepository(db),
		application.WithPasswordCredentialRepository(gorminfra.NewPasswordCredentialRepository(db)),
		// Lower bcrypt cost to keep the test fast; production callers should
		// stick to bcrypt.DefaultCost.
		application.WithBcryptCost(bcrypt.MinCost),
	)
}

func TestRegisterAndVerifyPassword(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSQLiteAuthService(t)

	agent, cred, account, err := svc.RegisterPassword(ctx, "Alice@example.com", "Alice", "hunter2")
	if err != nil {
		t.Fatalf("RegisterPassword: %v", err)
	}
	if agent == nil || cred == nil || account == nil {
		t.Fatalf("expected non-nil agent/cred/account, got %v %v %v", agent, cred, account)
	}
	if cred.Provider() != entities.ProviderPassword {
		t.Errorf("Provider() = %q, want %q", cred.Provider(), entities.ProviderPassword)
	}
	if cred.ProviderUserID() != "alice@example.com" {
		t.Errorf("ProviderUserID() = %q, want lowercased email", cred.ProviderUserID())
	}

	// Successful verify with matching password.
	verifiedAgent, verifiedCred, verifiedAccount, err := svc.VerifyPassword(ctx, "alice@example.com", "hunter2")
	if err != nil {
		t.Fatalf("VerifyPassword (success): %v", err)
	}
	if verifiedAgent.GetID() != agent.GetID() {
		t.Errorf("agent ID = %q, want %q", verifiedAgent.GetID(), agent.GetID())
	}
	if verifiedCred.GetID() != cred.GetID() {
		t.Errorf("cred ID = %q, want %q", verifiedCred.GetID(), cred.GetID())
	}
	if verifiedAccount == nil || verifiedAccount.GetID() != account.GetID() {
		t.Errorf("account mismatch: %+v vs %+v", verifiedAccount, account)
	}

	// Email is normalized — different case still matches.
	if _, _, _, err := svc.VerifyPassword(ctx, "ALICE@EXAMPLE.COM", "hunter2"); err != nil {
		t.Errorf("VerifyPassword with uppercase email failed: %v", err)
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSQLiteAuthService(t)

	if _, _, _, err := svc.RegisterPassword(ctx, "alice@example.com", "Alice", "hunter2"); err != nil {
		t.Fatalf("RegisterPassword: %v", err)
	}

	_, _, _, err := svc.VerifyPassword(ctx, "alice@example.com", "wrong-password")
	if !errors.Is(err, application.ErrInvalidPassword) {
		t.Errorf("error = %v, want ErrInvalidPassword", err)
	}
}

func TestVerifyPassword_UnknownEmail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSQLiteAuthService(t)

	_, _, _, err := svc.VerifyPassword(ctx, "ghost@example.com", "anything")
	if !errors.Is(err, application.ErrInvalidPassword) {
		t.Errorf("error = %v, want ErrInvalidPassword (not a different sentinel — anti-enumeration)", err)
	}
}

func TestRegisterPassword_DuplicateEmail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSQLiteAuthService(t)

	if _, _, _, err := svc.RegisterPassword(ctx, "alice@example.com", "Alice", "hunter2"); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, _, _, err := svc.RegisterPassword(ctx, "ALICE@example.com", "Alice2", "hunter3")
	if !errors.Is(err, application.ErrEmailAlreadyTaken) {
		t.Errorf("error = %v, want ErrEmailAlreadyTaken", err)
	}
}

func TestUpdatePassword(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSQLiteAuthService(t)

	agent, _, _, err := svc.RegisterPassword(ctx, "alice@example.com", "Alice", "hunter2")
	if err != nil {
		t.Fatalf("RegisterPassword: %v", err)
	}

	if err := svc.UpdatePassword(ctx, agent.GetID(), "wrong", "newpass1"); !errors.Is(err, application.ErrInvalidPassword) {
		t.Errorf("UpdatePassword with wrong old password = %v, want ErrInvalidPassword", err)
	}

	if err := svc.UpdatePassword(ctx, agent.GetID(), "hunter2", "newpass1"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}
	if _, _, _, err := svc.VerifyPassword(ctx, "alice@example.com", "hunter2"); !errors.Is(err, application.ErrInvalidPassword) {
		t.Errorf("expected old password to no longer verify, got %v", err)
	}
	if _, _, _, err := svc.VerifyPassword(ctx, "alice@example.com", "newpass1"); err != nil {
		t.Errorf("new password failed to verify: %v", err)
	}
}

func TestUpdatePassword_MissingPasswordCredential(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSQLiteAuthService(t)

	if err := svc.UpdatePassword(ctx, "missing-agent", "old", "new"); !errors.Is(err, application.ErrPasswordCredentialMissing) {
		t.Errorf("error = %v, want ErrPasswordCredentialMissing", err)
	}
}

func TestImportPasswordCredential(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSQLiteAuthService(t)

	// Seed an agent + personal account using RegisterPassword for a
	// different email so we have foreign keys to bind the import to.
	owner, _, ownerAccount, err := svc.RegisterPassword(ctx, "owner@example.com", "Owner", "ownerpass")
	if err != nil {
		t.Fatalf("seed RegisterPassword: %v", err)
	}

	// Hash a legacy password externally; bcrypt MinCost keeps the test fast.
	legacyHash, err := bcrypt.GenerateFromPassword([]byte("legacy-pass"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}

	// Import a password credential against the existing agent.
	if err := svc.ImportPasswordCredential(ctx, "legacy@example.com", "Legacy", string(legacyHash), owner.GetID(), ownerAccount.GetID()); err != nil {
		t.Fatalf("ImportPasswordCredential: %v", err)
	}

	// VerifyPassword against the imported hash succeeds without rehashing.
	verifiedAgent, _, _, err := svc.VerifyPassword(ctx, "legacy@example.com", "legacy-pass")
	if err != nil {
		t.Fatalf("VerifyPassword (imported): %v", err)
	}
	if verifiedAgent.GetID() != owner.GetID() {
		t.Errorf("imported credential resolved to agent %q, want %q", verifiedAgent.GetID(), owner.GetID())
	}

	// Idempotent: re-importing returns nil without error.
	if err := svc.ImportPasswordCredential(ctx, "legacy@example.com", "Legacy", string(legacyHash), owner.GetID(), ownerAccount.GetID()); err != nil {
		t.Errorf("re-import should be idempotent, got %v", err)
	}
}

func TestImportPasswordCredential_UnknownAgent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSQLiteAuthService(t)

	hash, _ := bcrypt.GenerateFromPassword([]byte("legacy-pass"), bcrypt.MinCost)
	err := svc.ImportPasswordCredential(ctx, "legacy@example.com", "Legacy", string(hash), "agent-does-not-exist", "")
	if err == nil || !strings.Contains(err.Error(), "agent agent-does-not-exist not found") {
		t.Errorf("expected agent-not-found error, got %v", err)
	}
}

func TestVerifyPassword_DeactivatedCredential(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := gorminfra.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	credRepo := gorminfra.NewCredentialRepository(db)
	svc := application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{},
		gorminfra.NewAgentRepository(db),
		credRepo,
		gorminfra.NewAuthSessionRepository(db),
		gorminfra.NewAccountRepository(db),
		application.WithPasswordCredentialRepository(gorminfra.NewPasswordCredentialRepository(db)),
		application.WithBcryptCost(bcrypt.MinCost),
	)

	_, cred, _, err := svc.RegisterPassword(ctx, "alice@example.com", "Alice", "hunter2")
	if err != nil {
		t.Fatalf("RegisterPassword: %v", err)
	}
	if err := cred.Deactivate(); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if err := credRepo.Save(ctx, cred); err != nil {
		t.Fatalf("Save deactivated credential: %v", err)
	}

	if _, _, _, err := svc.VerifyPassword(ctx, "alice@example.com", "hunter2"); !errors.Is(err, application.ErrInvalidPassword) {
		t.Errorf("expected ErrInvalidPassword for deactivated credential, got %v", err)
	}
}

func TestRegisterPasswordThenIssueJWT(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := gorminfra.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa: %v", err)
	}
	jwtSvc := authjwt.NewRSAJWTService(authjwt.WithSigningKey(rsaKey))

	svc := application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{},
		gorminfra.NewAgentRepository(db),
		gorminfra.NewCredentialRepository(db),
		gorminfra.NewAuthSessionRepository(db),
		gorminfra.NewAccountRepository(db),
		application.WithPasswordCredentialRepository(gorminfra.NewPasswordCredentialRepository(db)),
		application.WithJWTService(jwtSvc),
		application.WithBcryptCost(bcrypt.MinCost),
	)

	agent, _, account, err := svc.RegisterPassword(ctx, "alice@example.com", "Alice", "hunter2")
	if err != nil {
		t.Fatalf("RegisterPassword: %v", err)
	}

	verifiedAgent, _, _, err := svc.VerifyPassword(ctx, "alice@example.com", "hunter2")
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if verifiedAgent.GetID() != agent.GetID() {
		t.Fatalf("agent mismatch")
	}

	token, err := svc.IssueIdentityToken(ctx, verifiedAgent, account.GetID())
	if err != nil {
		t.Fatalf("IssueIdentityToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty JWT")
	}
	claims, err := jwtSvc.ValidateToken(ctx, token)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if claims.AgentID != agent.GetID() {
		t.Errorf("claims.AgentID = %q, want %q", claims.AgentID, agent.GetID())
	}
}

func TestUpdatePassword_WithEventStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := gorminfra.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store, err := esinfra.NewGormEventStore(db)
	if err != nil {
		t.Fatalf("event store: %v", err)
	}

	svc := application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{},
		gorminfra.NewAgentRepository(db),
		gorminfra.NewCredentialRepository(db),
		gorminfra.NewAuthSessionRepository(db),
		gorminfra.NewAccountRepository(db),
		application.WithPasswordCredentialRepository(gorminfra.NewPasswordCredentialRepository(db)),
		application.WithEventStore(store),
		application.WithBcryptCost(bcrypt.MinCost),
	)

	agent, _, _, err := svc.RegisterPassword(ctx, "alice@example.com", "Alice", "hunter2")
	if err != nil {
		t.Fatalf("RegisterPassword: %v", err)
	}

	// UpdatePassword on a rehydrated aggregate must not conflict with the
	// optimistic-concurrency check on the event store — see the comment in
	// UpdatePassword. This test fails with "expected version 0" if the
	// implementation regresses to using a UoW.
	if err := svc.UpdatePassword(ctx, agent.GetID(), "hunter2", "newpass1"); err != nil {
		t.Fatalf("UpdatePassword with event store: %v", err)
	}

	if _, _, _, err := svc.VerifyPassword(ctx, "alice@example.com", "newpass1"); err != nil {
		t.Errorf("verify after update failed: %v", err)
	}
}

func TestDummyBcryptHash_IsValid(t *testing.T) {
	t.Parallel()
	// Asserts that the constant in authentication_service.go parses as a
	// real bcrypt hash so verifyPassword performs the timing-equalizing
	// CompareHashAndPassword cycle. The expected error is the mismatch
	// sentinel — not a parse/format error.
	svc := newSQLiteAuthService(t)
	ctx := context.Background()

	// Calling VerifyPassword with no registered credentials hits the
	// dummy-hash branch. We do not assert on timing, just on the returned
	// error: it must be ErrInvalidPassword (not wrapped/unrelated).
	if _, _, _, err := svc.VerifyPassword(ctx, "ghost@example.com", "any"); !errors.Is(err, application.ErrInvalidPassword) {
		t.Errorf("error = %v, want ErrInvalidPassword (dummy hash may be malformed)", err)
	}
}

func TestPasswordSupportNotConfigured(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Service WITHOUT WithPasswordCredentialRepository.
	svc, _ := newTestService()

	if _, _, _, err := svc.RegisterPassword(ctx, "a@b.com", "A", "p"); !errors.Is(err, application.ErrPasswordSupportNotConfigured) {
		t.Errorf("RegisterPassword: %v", err)
	}
	if _, _, _, err := svc.VerifyPassword(ctx, "a@b.com", "p"); !errors.Is(err, application.ErrPasswordSupportNotConfigured) {
		t.Errorf("VerifyPassword: %v", err)
	}
	if err := svc.ImportPasswordCredential(ctx, "a@b.com", "A", "h", "agent", ""); !errors.Is(err, application.ErrPasswordSupportNotConfigured) {
		t.Errorf("ImportPasswordCredential: %v", err)
	}
	if err := svc.UpdatePassword(ctx, "agent", "old", "new"); !errors.Is(err, application.ErrPasswordSupportNotConfigured) {
		t.Errorf("UpdatePassword: %v", err)
	}
}
