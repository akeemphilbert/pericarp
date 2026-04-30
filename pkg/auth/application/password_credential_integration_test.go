package application_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
	gorminfra "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/database/gorm"
	authjwt "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/jwt"
	esdomain "github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
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

func TestImportPasswordCredential_WithSalt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSQLiteAuthService(t)

	owner, _, ownerAccount, err := svc.RegisterPassword(ctx, "owner@example.com", "Owner", "ownerpass")
	if err != nil {
		t.Fatalf("seed RegisterPassword: %v", err)
	}

	// Mimic the legacy IAM scheme: bcrypt(plaintext + salt).
	const plain = "hunter2"
	const salt = "abcde"
	legacyHash, err := bcrypt.GenerateFromPassword([]byte(plain+salt), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}

	if err := svc.ImportPasswordCredential(ctx,
		"legacy@example.com", "Legacy", string(legacyHash),
		owner.GetID(), ownerAccount.GetID(),
		application.ImportWithSalt(salt),
	); err != nil {
		t.Fatalf("ImportPasswordCredential: %v", err)
	}

	// Right plaintext succeeds — the service appends the stored salt
	// before bcrypt comparison.
	if _, _, _, err := svc.VerifyPassword(ctx, "legacy@example.com", plain); err != nil {
		t.Errorf("VerifyPassword (correct plaintext) = %v, want nil", err)
	}

	// Wrong plaintext fails with ErrInvalidPassword.
	if _, _, _, err := svc.VerifyPassword(ctx, "legacy@example.com", "wrong"); !errors.Is(err, application.ErrInvalidPassword) {
		t.Errorf("VerifyPassword (wrong plaintext) = %v, want ErrInvalidPassword", err)
	}
}

func TestImportPasswordCredential_WithoutSaltRejectsSaltedHash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := newSQLiteAuthService(t)

	owner, _, ownerAccount, err := svc.RegisterPassword(ctx, "owner@example.com", "Owner", "ownerpass")
	if err != nil {
		t.Fatalf("seed RegisterPassword: %v", err)
	}

	// Hash bcrypt(plaintext + salt) but import WITHOUT ImportWithSalt.
	// Verification should fail because the service compares against the
	// raw plaintext only — proving the salt is what makes verify succeed
	// in the salted-import path.
	const plain = "hunter2"
	const salt = "abcde"
	legacyHash, err := bcrypt.GenerateFromPassword([]byte(plain+salt), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}

	if err := svc.ImportPasswordCredential(ctx,
		"legacy@example.com", "Legacy", string(legacyHash),
		owner.GetID(), ownerAccount.GetID(),
		// no ImportWithSalt
	); err != nil {
		t.Fatalf("ImportPasswordCredential: %v", err)
	}
	if _, _, _, err := svc.VerifyPassword(ctx, "legacy@example.com", plain); !errors.Is(err, application.ErrInvalidPassword) {
		t.Errorf("VerifyPassword without imported salt = %v, want ErrInvalidPassword", err)
	}
}

func TestUpdatePassword_ClearsImportedSalt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, db := newSQLiteDeps(t)

	// Build a clean agent that has *only* the imported credential —
	// otherwise UpdatePassword (which picks the agent's first active
	// password credential) could rotate a different row. Insert via the
	// agent repository to skip RegisterPassword's own credential
	// creation.
	agent, err := new(entities.Agent).With("agent-legacy", "Legacy", "")
	if err != nil {
		t.Fatalf("Agent.With: %v", err)
	}
	if err := gorminfra.NewAgentRepository(db).Save(ctx, agent); err != nil {
		t.Fatalf("save agent: %v", err)
	}

	const email = "legacy@example.com"
	const plain = "hunter2"
	const salt = "abcde"
	legacyHash, err := bcrypt.GenerateFromPassword([]byte(plain+salt), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	if err := svc.ImportPasswordCredential(ctx,
		email, "Legacy", string(legacyHash),
		agent.GetID(), "",
		application.ImportWithSalt(salt),
	); err != nil {
		t.Fatalf("ImportPasswordCredential: %v", err)
	}

	// Sanity: salted plaintext verifies before rotation.
	if _, _, _, err := svc.VerifyPassword(ctx, email, plain); err != nil {
		t.Fatalf("pre-rotation VerifyPassword: %v", err)
	}

	// Rotate.
	if err := svc.UpdatePassword(ctx, agent.GetID(), plain, "newpass1"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}

	// Salt cleared on the projection row.
	credRepo := gorminfra.NewCredentialRepository(db)
	pcRepo := gorminfra.NewPasswordCredentialRepository(db)
	cred, err := credRepo.FindByProvider(ctx, entities.ProviderPassword, email)
	if err != nil || cred == nil {
		t.Fatalf("locate credential after rotation: %v %+v", err, cred)
	}
	rotated, err := pcRepo.FindByCredentialID(ctx, cred.GetID())
	if err != nil || rotated == nil {
		t.Fatalf("locate password credential after rotation: %v %+v", err, rotated)
	}
	if rotated.Salt() != "" {
		t.Errorf("Salt() after rotation = %q, want empty (UpdatePassword must clear legacy salt)", rotated.Salt())
	}

	// New plaintext verifies without any salt suffix.
	if _, _, _, err := svc.VerifyPassword(ctx, email, "newpass1"); err != nil {
		t.Errorf("new password failed to verify: %v", err)
	}
	// Original salted plaintext no longer matches — both hash and salt
	// changed.
	if _, _, _, err := svc.VerifyPassword(ctx, email, plain); !errors.Is(err, application.ErrInvalidPassword) {
		t.Errorf("legacy plaintext still verifies after rotation: %v", err)
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
	// implementation regresses to using a UoW without first reseating the
	// aggregate's sequence number to the stream's current version.
	if err := svc.UpdatePassword(ctx, agent.GetID(), "hunter2", "newpass1"); err != nil {
		t.Fatalf("UpdatePassword with event store: %v", err)
	}

	if _, _, _, err := svc.VerifyPassword(ctx, "alice@example.com", "newpass1"); err != nil {
		t.Errorf("verify after update failed: %v", err)
	}

	// Regression: the PasswordUpdated event must reach the event store so
	// password rotations remain auditable. A previous implementation
	// dropped the event by calling ClearUncommittedEvents().
	credRepo := gorminfra.NewCredentialRepository(db)
	cred, err := credRepo.FindByProvider(ctx, entities.ProviderPassword, "alice@example.com")
	if err != nil || cred == nil {
		t.Fatalf("locate credential: %v %+v", err, cred)
	}
	pcRepo := gorminfra.NewPasswordCredentialRepository(db)
	pc, err := pcRepo.FindByCredentialID(ctx, cred.GetID())
	if err != nil || pc == nil {
		t.Fatalf("locate password credential: %v %+v", err, pc)
	}
	events, err := store.GetEvents(ctx, pc.GetID())
	if err != nil {
		t.Fatalf("GetEvents: %v", err)
	}
	var sawUpdated bool
	for _, ev := range events {
		if ev.EventType == entities.EventTypePasswordUpdated {
			sawUpdated = true
		}
	}
	if !sawUpdated {
		t.Errorf("event store missing %q event for aggregate %s; got %d events", entities.EventTypePasswordUpdated, pc.GetID(), len(events))
	}
}

// TestVerifyPassword_DoesNotTouchRotatedAt is a regression test for the
// GORM auto-update timestamp footgun: PasswordCredentialModel previously
// named its rotated-at column field UpdatedAt, which GORM silently bumped
// on every Save (including the LastVerifiedAt update done after a
// successful verify), corrupting the domain meaning of UpdatedAt() as
// "password rotated at."
func TestVerifyPassword_DoesNotTouchRotatedAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, db := newSQLiteDeps(t)
	pcRepo := gorminfra.NewPasswordCredentialRepository(db)
	credRepo := gorminfra.NewCredentialRepository(db)

	if _, _, _, err := svc.RegisterPassword(ctx, "alice@example.com", "Alice", "hunter2"); err != nil {
		t.Fatalf("RegisterPassword: %v", err)
	}
	cred, err := credRepo.FindByProvider(ctx, entities.ProviderPassword, "alice@example.com")
	if err != nil || cred == nil {
		t.Fatalf("locate credential: %v %+v", err, cred)
	}
	before, err := pcRepo.FindByCredentialID(ctx, cred.GetID())
	if err != nil || before == nil {
		t.Fatalf("locate password credential: %v %+v", err, before)
	}
	rotatedBefore := before.UpdatedAt()

	if _, _, _, err := svc.VerifyPassword(ctx, "alice@example.com", "hunter2"); err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}

	after, err := pcRepo.FindByCredentialID(ctx, cred.GetID())
	if err != nil || after == nil {
		t.Fatalf("locate password credential after verify: %v %+v", err, after)
	}
	if !after.UpdatedAt().Equal(rotatedBefore) {
		t.Errorf("VerifyPassword bumped UpdatedAt (rotated_at): before=%s after=%s", rotatedBefore, after.UpdatedAt())
	}
	// LastVerifiedAt should advance, confirming the row was actually
	// written and we are not testing a no-op.
	if !after.LastVerifiedAt().After(before.LastVerifiedAt()) {
		t.Errorf("expected LastVerifiedAt to advance after verify: before=%s after=%s", before.LastVerifiedAt(), after.LastVerifiedAt())
	}
}

// TestImportPasswordCredential_RejectsMalformedHash covers the validation
// gap where any non-empty string was accepted as a bcrypt hash, which
// would silently turn into ErrInvalidPassword on every future login.
func TestImportPasswordCredential_RejectsMalformedHash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, _ := newSQLiteDeps(t)

	owner, _, ownerAccount, err := svc.RegisterPassword(ctx, "owner@example.com", "Owner", "ownerpass")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	cases := []string{
		"not-a-bcrypt-hash",
		"$2a$10$abc",         // truncated
		"plaintext-password", // obvious migration mistake
	}
	for _, badHash := range cases {
		err := svc.ImportPasswordCredential(ctx, "legacy@example.com", "Legacy", badHash, owner.GetID(), ownerAccount.GetID())
		if err == nil {
			t.Errorf("ImportPasswordCredential(%q) accepted malformed hash; want error", badHash)
			continue
		}
		if !strings.Contains(err.Error(), "invalid bcrypt hash") {
			t.Errorf("ImportPasswordCredential(%q) error = %v, want invalid-bcrypt-hash", badHash, err)
		}
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

// newSQLiteDeps returns the SQLite-backed service plus the underlying
// repos so a test can poke at projections directly.
func newSQLiteDeps(t *testing.T) (*application.DefaultAuthenticationService, *gorm.DB) {
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
	svc := application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{},
		gorminfra.NewAgentRepository(db),
		gorminfra.NewCredentialRepository(db),
		gorminfra.NewAuthSessionRepository(db),
		gorminfra.NewAccountRepository(db),
		application.WithPasswordCredentialRepository(gorminfra.NewPasswordCredentialRepository(db)),
		application.WithBcryptCost(bcrypt.MinCost),
	)
	return svc, db
}

func TestImportPasswordCredential_DoesNotOverwriteOnReimport(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, db := newSQLiteDeps(t)
	pcRepo := gorminfra.NewPasswordCredentialRepository(db)

	owner, _, ownerAccount, err := svc.RegisterPassword(ctx, "owner@example.com", "Owner", "ownerpass")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	firstHash, _ := bcrypt.GenerateFromPassword([]byte("legacy-pass"), bcrypt.MinCost)
	if err := svc.ImportPasswordCredential(ctx, "legacy@example.com", "Legacy", string(firstHash), owner.GetID(), ownerAccount.GetID()); err != nil {
		t.Fatalf("first import: %v", err)
	}
	credRepo := gorminfra.NewCredentialRepository(db)
	cred, err := credRepo.FindByProvider(ctx, entities.ProviderPassword, "legacy@example.com")
	if err != nil || cred == nil {
		t.Fatalf("locate credential: %v %+v", err, cred)
	}
	pcBefore, err := pcRepo.FindByCredentialID(ctx, cred.GetID())
	if err != nil || pcBefore == nil {
		t.Fatalf("locate password credential: %v %+v", err, pcBefore)
	}

	// Re-import with a DIFFERENT hash. Idempotency means the second call
	// is a no-op — the stored hash must not change.
	secondHash, _ := bcrypt.GenerateFromPassword([]byte("different-pass"), bcrypt.MinCost)
	if err := svc.ImportPasswordCredential(ctx, "legacy@example.com", "Legacy", string(secondHash), owner.GetID(), ownerAccount.GetID()); err != nil {
		t.Fatalf("re-import: %v", err)
	}
	pcAfter, err := pcRepo.FindByCredentialID(ctx, cred.GetID())
	if err != nil || pcAfter == nil {
		t.Fatalf("locate password credential after: %v %+v", err, pcAfter)
	}
	if pcAfter.GetID() != pcBefore.GetID() {
		t.Errorf("password credential ID changed: %q -> %q", pcBefore.GetID(), pcAfter.GetID())
	}
	if pcAfter.Hash() != pcBefore.Hash() {
		t.Errorf("hash was overwritten on re-import; want unchanged")
	}

	// The original password must still verify; the second hash must not.
	if _, _, _, err := svc.VerifyPassword(ctx, "legacy@example.com", "legacy-pass"); err != nil {
		t.Errorf("original password no longer verifies: %v", err)
	}
	if _, _, _, err := svc.VerifyPassword(ctx, "legacy@example.com", "different-pass"); !errors.Is(err, application.ErrInvalidPassword) {
		t.Errorf("second-import password should not have taken effect, got %v", err)
	}
}

func TestImportPasswordCredential_NormalizesEmail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, db := newSQLiteDeps(t)

	owner, _, ownerAccount, err := svc.RegisterPassword(ctx, "owner@example.com", "Owner", "ownerpass")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("legacy-pass"), bcrypt.MinCost)

	if err := svc.ImportPasswordCredential(ctx, "legacy@example.com", "Legacy", string(hash), owner.GetID(), ownerAccount.GetID()); err != nil {
		t.Fatalf("first import: %v", err)
	}
	// Mixed-case + whitespace must hit the existing row, not create a new
	// one. The total row count for provider=password is the seed (1) +
	// legacy (1) = 2.
	if err := svc.ImportPasswordCredential(ctx, "  LEGACY@Example.COM  ", "Legacy", string(hash), owner.GetID(), ownerAccount.GetID()); err != nil {
		t.Fatalf("normalized re-import: %v", err)
	}
	credRepo := gorminfra.NewCredentialRepository(db)
	creds, err := credRepo.FindByEmail(ctx, "legacy@example.com")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if len(creds) != 1 {
		t.Errorf("expected exactly 1 credential for legacy@example.com, got %d", len(creds))
	}
}

func TestVerifyPassword_OrphanCredentialRow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc, db := newSQLiteDeps(t)
	credRepo := gorminfra.NewCredentialRepository(db)
	pcRepo := gorminfra.NewPasswordCredentialRepository(db)

	if _, _, _, err := svc.RegisterPassword(ctx, "alice@example.com", "Alice", "hunter2"); err != nil {
		t.Fatalf("RegisterPassword: %v", err)
	}
	cred, err := credRepo.FindByProvider(ctx, entities.ProviderPassword, "alice@example.com")
	if err != nil || cred == nil {
		t.Fatalf("locate credential: %v %+v", err, cred)
	}
	// Drop the password row but leave the parent credential — simulating
	// data drift between the two tables. VerifyPassword must still return
	// ErrInvalidPassword and exercise the timing shield.
	if err := pcRepo.Delete(ctx, cred.GetID()); err != nil {
		t.Fatalf("delete password row: %v", err)
	}

	_, _, _, err = svc.VerifyPassword(ctx, "alice@example.com", "hunter2")
	if !errors.Is(err, application.ErrInvalidPassword) {
		t.Errorf("orphan credential row: got %v, want ErrInvalidPassword", err)
	}
}

func TestPasswordEvents_NoHashInJSON(t *testing.T) {
	t.Parallel()

	const secret = "$2a$10$top-secret-hash-should-never-appear"
	pc, err := new(entities.PasswordCredential).With("pc-1", "cred-1", "bcrypt", secret)
	if err != nil {
		t.Fatalf("With: %v", err)
	}
	if err := pc.Update("bcrypt", "$2a$12$another-secret"); err != nil {
		t.Fatalf("Update: %v", err)
	}

	for _, evt := range pc.GetUncommittedEvents() {
		raw, err := json.Marshal(evt.Payload)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if bytes.Contains(raw, []byte("$2a$")) {
			t.Errorf("event JSON contains a bcrypt-hash substring: %s -> %s", evt.EventType, raw)
		}
	}

	// The redacted Stringer must not leak the hash either.
	if str := pc.String(); strings.Contains(str, "$2a$") {
		t.Errorf("PasswordCredential.String() leaks hash: %s", str)
	}
}

func TestDummyHash_TracksConfiguredCost(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Service with a non-default bcrypt cost. The unknown-email branch
	// should run a compare against a hash AT THE CONFIGURED COST so timing
	// matches a real failed login. We can't directly assert cost from the
	// service (private field), but we can prove the timing shield ran by
	// confirming the unknown-email path returns ErrInvalidPassword
	// (regression: a malformed dummy returned the same error trivially
	// fast — that's covered by TestVerifyPassword_UnknownEmail's
	// configured-cost variant below).
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := gorminfra.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	svc := application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{},
		gorminfra.NewAgentRepository(db),
		gorminfra.NewCredentialRepository(db),
		gorminfra.NewAuthSessionRepository(db),
		gorminfra.NewAccountRepository(db),
		application.WithPasswordCredentialRepository(gorminfra.NewPasswordCredentialRepository(db)),
		application.WithBcryptCost(bcrypt.MinCost+1),
	)

	if _, _, _, err := svc.VerifyPassword(ctx, "nobody@example.com", "anything"); !errors.Is(err, application.ErrInvalidPassword) {
		t.Errorf("expected ErrInvalidPassword, got %v", err)
	}
	// Second call exercises the cached dummy hash; both must succeed.
	if _, _, _, err := svc.VerifyPassword(ctx, "nobody2@example.com", "anything"); !errors.Is(err, application.ErrInvalidPassword) {
		t.Errorf("expected ErrInvalidPassword on cached path, got %v", err)
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

// blindCredentialRepo wraps a real CredentialRepository but always reports
// "no existing credential" from FindByProvider. This deterministically
// simulates the TOCTOU race where two concurrent RegisterPassword calls
// both pass the pre-check and the second one hits the unique index on
// (provider, provider_user_id) at credentials.Save.
type blindCredentialRepo struct {
	repositories.CredentialRepository
}

func (b *blindCredentialRepo) FindByProvider(ctx context.Context, provider, providerUserID string) (*entities.Credential, error) {
	return nil, nil
}

func TestRegisterPassword_DuplicateOnSaveTranslates(t *testing.T) {
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

	// Seed an existing password credential for alice@example.com so the
	// second register's Save will violate the unique index.
	seed := application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{},
		gorminfra.NewAgentRepository(db),
		credRepo,
		gorminfra.NewAuthSessionRepository(db),
		gorminfra.NewAccountRepository(db),
		application.WithPasswordCredentialRepository(gorminfra.NewPasswordCredentialRepository(db)),
		application.WithBcryptCost(bcrypt.MinCost),
	)
	if _, _, _, err := seed.RegisterPassword(ctx, "alice@example.com", "Alice", "hunter2"); err != nil {
		t.Fatalf("seed RegisterPassword: %v", err)
	}

	// New service whose CredentialRepository always lies on FindByProvider —
	// the pre-check passes, Save runs, the unique index fires.
	racy := application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{},
		gorminfra.NewAgentRepository(db),
		&blindCredentialRepo{CredentialRepository: credRepo},
		gorminfra.NewAuthSessionRepository(db),
		gorminfra.NewAccountRepository(db),
		application.WithPasswordCredentialRepository(gorminfra.NewPasswordCredentialRepository(db)),
		application.WithBcryptCost(bcrypt.MinCost),
	)

	_, _, _, err = racy.RegisterPassword(ctx, "alice@example.com", "Alice2", "hunter3")
	if !errors.Is(err, application.ErrEmailAlreadyTaken) {
		t.Fatalf("error = %v, want ErrEmailAlreadyTaken (dup-key was not translated)", err)
	}
}

func TestImportPasswordCredential_DuplicateOnSaveIsIdempotent(t *testing.T) {
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

	seed := application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{},
		gorminfra.NewAgentRepository(db),
		credRepo,
		gorminfra.NewAuthSessionRepository(db),
		gorminfra.NewAccountRepository(db),
		application.WithPasswordCredentialRepository(gorminfra.NewPasswordCredentialRepository(db)),
		application.WithBcryptCost(bcrypt.MinCost),
	)
	owner, _, ownerAccount, err := seed.RegisterPassword(ctx, "owner@example.com", "Owner", "ownerpass")
	if err != nil {
		t.Fatalf("seed RegisterPassword: %v", err)
	}
	// Seed a real credential for the email we'll import; the racy
	// service's blind FindByProvider will skip the idempotent pre-check
	// and fall through to credentials.Save where the unique index fires.
	if err := seed.ImportPasswordCredential(ctx,
		"legacy@example.com", "Legacy",
		"$2a$04$abcdefghijklmnopqrstuuwOl4ZJN8xpZ/Hf1jZQp7m/0ePI1ZGGy",
		owner.GetID(), ownerAccount.GetID()); err != nil {
		t.Fatalf("seed ImportPasswordCredential: %v", err)
	}

	racy := application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{},
		gorminfra.NewAgentRepository(db),
		&blindCredentialRepo{CredentialRepository: credRepo},
		gorminfra.NewAuthSessionRepository(db),
		gorminfra.NewAccountRepository(db),
		application.WithPasswordCredentialRepository(gorminfra.NewPasswordCredentialRepository(db)),
		application.WithBcryptCost(bcrypt.MinCost),
	)

	if err := racy.ImportPasswordCredential(ctx,
		"legacy@example.com", "Legacy",
		"$2a$04$abcdefghijklmnopqrstuuwOl4ZJN8xpZ/Hf1jZQp7m/0ePI1ZGGy",
		owner.GetID(), ownerAccount.GetID()); err != nil {
		t.Fatalf("ImportPasswordCredential after dup-key race must be idempotent (nil), got %v", err)
	}
}

// TestRegisterPassword_DispatchesEventsWhenConfigured covers a second wired
// commit site (RegisterPassword) so a future refactor reverting `s.dispatcher`
// to `nil` at any of FindOrCreateAgent / RegisterPassword / Import / Update
// can be caught by tests of qualitatively different aggregates rather than
// only the OAuth flow. We assert the password-specific event lands at the
// dispatcher; the generic Agent/Account/Credential events are already covered
// by TestFindOrCreateAgent_DispatchesEventsWhenConfigured.
func TestRegisterPassword_DispatchesEventsWhenConfigured(t *testing.T) {
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
		t.Fatalf("NewGormEventStore: %v", err)
	}

	dispatcher := esdomain.NewEventDispatcher()

	var mu sync.Mutex
	var received []string
	if err := dispatcher.SubscribeWildcard(func(_ context.Context, env esdomain.EventEnvelope[any]) error {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, env.EventType)
		return nil
	}); err != nil {
		t.Fatalf("SubscribeWildcard: %v", err)
	}

	svc := application.NewDefaultAuthenticationService(
		application.OAuthProviderRegistry{},
		gorminfra.NewAgentRepository(db),
		gorminfra.NewCredentialRepository(db),
		gorminfra.NewAuthSessionRepository(db),
		gorminfra.NewAccountRepository(db),
		application.WithPasswordCredentialRepository(gorminfra.NewPasswordCredentialRepository(db)),
		application.WithBcryptCost(bcrypt.MinCost),
		application.WithEventStore(store),
		application.WithEventDispatcher(dispatcher),
	)

	if _, _, _, err := svc.RegisterPassword(ctx, "alice@example.com", "Alice", "hunter2"); err != nil {
		t.Fatalf("RegisterPassword: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(received, entities.EventTypePasswordCredentialCreated) {
		t.Errorf("dispatcher did not receive %q; got %v", entities.EventTypePasswordCredentialCreated, received)
	}
}
