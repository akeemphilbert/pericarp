package subscription_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/subscription"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var _ application.SubscriptionService = (*subscription.GORM)(nil)

func newGormTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&subscription.SubscriptionRecord{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestGORM_NoRecord_ReturnsNil(t *testing.T) {
	t.Parallel()

	db := newGormTestDB(t)
	g := subscription.NewGORM(db)

	claim, err := g.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim != nil {
		t.Errorf("expected nil claim, got %+v", claim)
	}
}

func TestGORM_AgentOnlyMatch(t *testing.T) {
	t.Parallel()

	db := newGormTestDB(t)
	expires := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	row := &subscription.SubscriptionRecord{
		AgentID:   "agent-1",
		AccountID: "",
		Status:    string(auth.SubscriptionStatusActive),
		Plan:      "pro",
		Provider:  "stripe",
		ExpiresAt: &expires,
		UpdatedAt: time.Now(),
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	g := subscription.NewGORM(db)
	claim, err := g.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim == nil {
		t.Fatal("expected claim")
	}
	if claim.Status != auth.SubscriptionStatusActive {
		t.Errorf("Status = %q, want active", claim.Status)
	}
	if claim.Plan != "pro" {
		t.Errorf("Plan = %q, want pro", claim.Plan)
	}
	if claim.Provider != "stripe" {
		t.Errorf("Provider = %q, want stripe (row value wins over fallback)", claim.Provider)
	}
	if !claim.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt = %v, want %v", claim.ExpiresAt, expires)
	}
}

func TestGORM_AccountScopedPreferredOverAgentOnly(t *testing.T) {
	// A paid subscription on the agent's personal account must not
	// automatically apply to a B2B account the agent also belongs to —
	// the account-scoped row wins when accountID is requested.
	t.Parallel()

	db := newGormTestDB(t)
	now := time.Now()
	rows := []*subscription.SubscriptionRecord{
		{
			AgentID: "agent-1", AccountID: "",
			Status: string(auth.SubscriptionStatusActive), Plan: "personal",
			UpdatedAt: now.Add(-time.Hour),
		},
		{
			AgentID: "agent-1", AccountID: "team-1",
			Status: string(auth.SubscriptionStatusInactive), Plan: "team_free",
			UpdatedAt: now,
		},
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	g := subscription.NewGORM(db)
	claim, err := g.GetSubscription(context.Background(), "agent-1", "team-1")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Plan != "team_free" {
		t.Errorf("Plan = %q, want team_free (account-scoped row wins over agent-only)", claim.Plan)
	}
	if claim.Status != auth.SubscriptionStatusInactive {
		// Pin the status to the team-scoped row so a regression that
		// blends results between rows would surface here.
		t.Errorf("Status = %q, want inactive (team row's status, not personal row's active)", claim.Status)
	}
}

func TestGORM_NonEmptyAccountWithNoMatch_ReturnsNil(t *testing.T) {
	// Critical invariant: a paid personal-account subscription must NOT
	// silently grant paid-tier access to a B2B/team account the same
	// agent belongs to. When accountID is non-empty and no
	// account-scoped row exists, the lookup returns (nil, nil) — it
	// does not fall back to the agent-only personal row.
	t.Parallel()

	db := newGormTestDB(t)
	row := &subscription.SubscriptionRecord{
		AgentID: "agent-1", AccountID: "",
		Status: string(auth.SubscriptionStatusActive), Plan: "personal",
		UpdatedAt: time.Now(),
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	g := subscription.NewGORM(db)
	claim, err := g.GetSubscription(context.Background(), "agent-1", "team-without-row")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim != nil {
		t.Errorf("expected nil claim, got %+v — personal subscription must not leak across account scope", claim)
	}
}

func TestGORM_LatestUpdatedAtWins(t *testing.T) {
	t.Parallel()

	db := newGormTestDB(t)
	now := time.Now()
	rows := []*subscription.SubscriptionRecord{
		{AgentID: "agent-1", Status: "active", Plan: "old", UpdatedAt: now.Add(-2 * time.Hour)},
		{AgentID: "agent-1", Status: "active", Plan: "new", UpdatedAt: now},
		{AgentID: "agent-1", Status: "active", Plan: "older", UpdatedAt: now.Add(-1 * time.Hour)},
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	g := subscription.NewGORM(db)
	claim, err := g.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Plan != "new" {
		t.Errorf("Plan = %q, want new (latest updated_at)", claim.Plan)
	}
}

func TestGORM_ProviderFallback(t *testing.T) {
	t.Parallel()

	db := newGormTestDB(t)
	row := &subscription.SubscriptionRecord{
		AgentID: "agent-1", Status: "active", Plan: "pro",
		UpdatedAt: time.Now(),
		// Provider intentionally empty
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	g := subscription.NewGORM(db, subscription.WithGORMProvider("internal"))
	claim, err := g.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Provider != "internal" {
		t.Errorf("Provider = %q, want internal (fallback applied when row provider is empty)", claim.Provider)
	}
}

func TestGORM_CustomTable(t *testing.T) {
	t.Parallel()

	db := newGormTestDB(t)
	if err := db.Exec(`CREATE TABLE custom_subs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_id TEXT,
		account_id TEXT,
		status TEXT,
		plan TEXT,
		provider TEXT,
		expires_at DATETIME,
		updated_at DATETIME
	)`).Error; err != nil {
		t.Fatalf("create custom table: %v", err)
	}
	if err := db.Exec(`INSERT INTO custom_subs (agent_id, account_id, status, plan, provider, updated_at) VALUES ('agent-1','','active','pro','stripe',?)`, time.Now()).Error; err != nil {
		t.Fatalf("seed custom: %v", err)
	}

	g := subscription.NewGORM(db, subscription.WithGORMTable("custom_subs"))
	claim, err := g.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim == nil || claim.Plan != "pro" {
		t.Errorf("claim = %+v, want plan=pro", claim)
	}
}

func TestGORM_ResolverEscapeHatch(t *testing.T) {
	t.Parallel()

	db := newGormTestDB(t)
	calledArgs := struct {
		agentID, accountID string
	}{}
	resolver := func(ctx context.Context, db *gorm.DB, agentID, accountID string) (*auth.SubscriptionClaim, error) {
		calledArgs.agentID = agentID
		calledArgs.accountID = accountID
		return &auth.SubscriptionClaim{
			Status:   auth.SubscriptionStatusActive,
			Plan:     "from-resolver",
			Provider: "custom",
		}, nil
	}

	g := subscription.NewGORM(db, subscription.WithGORMResolver(resolver))
	claim, err := g.GetSubscription(context.Background(), "agent-1", "team-1")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if calledArgs.agentID != "agent-1" || calledArgs.accountID != "team-1" {
		t.Errorf("resolver called with %+v, want {agent-1, team-1}", calledArgs)
	}
	if claim.Plan != "from-resolver" {
		t.Errorf("Plan = %q, want from-resolver", claim.Plan)
	}
}

func TestGORM_ResolverPropagatesError(t *testing.T) {
	t.Parallel()

	db := newGormTestDB(t)
	bang := errors.New("backend boom")
	g := subscription.NewGORM(db, subscription.WithGORMResolver(func(ctx context.Context, db *gorm.DB, _, _ string) (*auth.SubscriptionClaim, error) {
		return nil, bang
	}))
	_, err := g.GetSubscription(context.Background(), "agent-1", "")
	if !errors.Is(err, bang) {
		t.Errorf("err = %v, want %v wrapped", err, bang)
	}
}

func TestGORM_NilDB_Errors(t *testing.T) {
	t.Parallel()

	g := subscription.NewGORM(nil)
	if _, err := g.GetSubscription(context.Background(), "agent-1", ""); err == nil {
		t.Fatal("expected error for nil DB")
	}
}

func TestGORM_EmptyAgentID_Errors(t *testing.T) {
	t.Parallel()

	db := newGormTestDB(t)
	g := subscription.NewGORM(db)
	if _, err := g.GetSubscription(context.Background(), "", ""); err == nil {
		t.Fatal("expected error for empty agent ID")
	}
}

func TestGORM_NilExpiresAt_ZeroOnClaim_ActiveLifetime(t *testing.T) {
	t.Parallel()

	db := newGormTestDB(t)
	row := &subscription.SubscriptionRecord{
		AgentID: "agent-1", Status: "active", Plan: "lifetime",
		ExpiresAt: nil, UpdatedAt: time.Now(),
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	g := subscription.NewGORM(db)
	claim, err := g.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if !claim.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero for nil row.expires_at", claim.ExpiresAt)
	}
	if !claim.IsActive() {
		t.Error("IsActive() = false, want true for active lifetime claim with no expiry")
	}
}

func TestGORM_NullAccountID_MatchedInAgentOnlyLookup(t *testing.T) {
	// Schemas that store account_id as nullable rather than empty string
	// must still match the agent-only fallback. Seed with raw SQL because
	// the default *string SubscriptionRecord schema writes "" for the
	// zero value.
	t.Parallel()

	db := newGormTestDB(t)
	if err := db.Exec(
		"INSERT INTO subscriptions (agent_id, account_id, status, plan, updated_at) VALUES (?, NULL, ?, ?, ?)",
		"agent-1", "active", "pro", time.Now(),
	).Error; err != nil {
		t.Fatalf("seed via raw SQL: %v", err)
	}

	g := subscription.NewGORM(db)
	claim, err := g.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim == nil {
		t.Fatal("expected claim for NULL account_id row")
	}
	if claim.Plan != "pro" {
		t.Errorf("Plan = %q, want pro", claim.Plan)
	}
}

func TestGORM_NoMatchAtAll_ReturnsNil(t *testing.T) {
	// A different agent has a row; agent-1 has none. Both lookups must
	// miss and the function returns (nil, nil) without surfacing
	// gorm.ErrRecordNotFound to the caller.
	t.Parallel()

	db := newGormTestDB(t)
	row := &subscription.SubscriptionRecord{
		AgentID: "agent-other", AccountID: "team-1",
		Status: "active", Plan: "pro", UpdatedAt: time.Now(),
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	g := subscription.NewGORM(db)
	claim, err := g.GetSubscription(context.Background(), "agent-1", "team-1")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim != nil {
		t.Errorf("expected nil claim, got %+v", claim)
	}
}

func TestGORM_DBError_NotSwallowed(t *testing.T) {
	// Closing the underlying DB causes any subsequent query to return a
	// non-ErrRecordNotFound error. Adapter must propagate it (caller
	// distinguishes "no record" from "lookup failed").
	t.Parallel()

	db := newGormTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get *sql.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close DB: %v", err)
	}

	g := subscription.NewGORM(db)

	for _, accountID := range []string{"", "team-1"} {
		_, err := g.GetSubscription(context.Background(), "agent-1", accountID)
		if err == nil {
			t.Errorf("accountID=%q: expected error, got nil", accountID)
		}
	}
}

func TestGORM_RowProvider_WinsOverFallbackOption(t *testing.T) {
	// Pin the documented contract: when the row's provider column is
	// non-empty, WithGORMProvider's fallback is ignored.
	t.Parallel()

	db := newGormTestDB(t)
	row := &subscription.SubscriptionRecord{
		AgentID: "agent-1", Status: "active", Plan: "pro",
		Provider: "stripe", UpdatedAt: time.Now(),
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	g := subscription.NewGORM(db, subscription.WithGORMProvider("internal"))
	claim, err := g.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Provider != "stripe" {
		t.Errorf("Provider = %q, want stripe (row value must win over WithGORMProvider fallback)", claim.Provider)
	}
}

func TestGORM_EmptyAgentID_ResolverNotInvoked(t *testing.T) {
	// The empty-agentID guard must run before resolver dispatch — a
	// custom resolver should never see an empty agentID.
	t.Parallel()

	db := newGormTestDB(t)
	called := false
	g := subscription.NewGORM(db, subscription.WithGORMResolver(func(ctx context.Context, _ *gorm.DB, _, _ string) (*auth.SubscriptionClaim, error) {
		called = true
		return nil, nil
	}))
	if _, err := g.GetSubscription(context.Background(), "", "team-1"); err == nil {
		t.Fatal("expected error for empty agent ID")
	}
	if called {
		t.Error("resolver was invoked despite empty agentID")
	}
}

func TestGORM_InvalidStatusFromResolver_ReturnsError(t *testing.T) {
	// A buggy resolver returning a non-canonical status (wrong case,
	// typo, etc.) must not silently propagate into a JWT — the adapter
	// surfaces it as an error so it lands in the caller's log.
	t.Parallel()

	db := newGormTestDB(t)
	g := subscription.NewGORM(db, subscription.WithGORMResolver(func(_ context.Context, _ *gorm.DB, _, _ string) (*auth.SubscriptionClaim, error) {
		return &auth.SubscriptionClaim{Status: "ACTIVE"}, nil
	}))
	claim, err := g.GetSubscription(context.Background(), "agent-1", "")
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	if claim != nil {
		t.Errorf("expected nil claim on invalid status, got %+v", claim)
	}
}

func TestGORM_InvalidStatusFromRow_ReturnsError(t *testing.T) {
	// Same defense from the default-row path.
	t.Parallel()

	db := newGormTestDB(t)
	row := &subscription.SubscriptionRecord{
		AgentID: "agent-1", Status: "ACTIVE", Plan: "pro", UpdatedAt: time.Now(),
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	g := subscription.NewGORM(db)
	claim, err := g.GetSubscription(context.Background(), "agent-1", "")
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	if claim != nil {
		t.Errorf("expected nil claim, got %+v", claim)
	}
}
