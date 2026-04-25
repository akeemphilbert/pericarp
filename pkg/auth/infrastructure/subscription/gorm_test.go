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
}

func TestGORM_NoAccountMatch_FallsBackToAgentOnly(t *testing.T) {
	// When the requested account has no row but the agent has an
	// agent-only row, the fallback returns it. (B2B account inherits
	// the agent's personal subscription only when no team-scoped row
	// exists.)
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
	if claim == nil {
		t.Fatal("expected agent-only fallback claim")
	}
	if claim.Plan != "personal" {
		t.Errorf("Plan = %q, want personal", claim.Plan)
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

func TestGORM_NilExpiresAt_ZeroOnClaim(t *testing.T) {
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
}
