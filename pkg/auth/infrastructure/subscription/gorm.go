package subscription

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"gorm.io/gorm"
)

const (
	gormDefaultTable = "subscriptions"
)

// SubscriptionRecord is the default row shape the GORM adapter reads from.
// Consumer services that already own a billing projection can either
// AutoMigrate this struct into their schema or override the table name via
// WithGORMTable; for fully custom schemas use WithGORMResolver.
type SubscriptionRecord struct {
	ID        uint       `gorm:"primaryKey"`
	AgentID   string     `gorm:"size:64;index:idx_sub_agent;not null"`
	AccountID string     `gorm:"size:64;index:idx_sub_agent_account"`
	Status    string     `gorm:"size:32;not null"`
	Plan      string     `gorm:"size:128"`
	Provider  string     `gorm:"size:64"`
	ExpiresAt *time.Time
	UpdatedAt time.Time
}

// TableName returns the default table name. Overridden in queries via
// WithGORMTable rather than tag-based reflection so the same struct can
// back differently-named tables in different services.
func (SubscriptionRecord) TableName() string { return gormDefaultTable }

// Resolver is the fully-custom-query escape hatch. When provided via
// WithGORMResolver the adapter delegates entirely to the function and
// ignores the default schema; that lets services with bespoke billing
// projections plug in arbitrary SQL/GORM joins without forking the
// adapter.
type Resolver func(ctx context.Context, db *gorm.DB, agentID, accountID string) (*auth.SubscriptionClaim, error)

// GORMOption configures a GORM adapter.
type GORMOption func(*GORM)

// WithGORMTable overrides the table name the default resolver queries.
// Only meaningful when no custom resolver is configured.
func WithGORMTable(name string) GORMOption {
	return func(g *GORM) {
		if name != "" {
			g.table = name
		}
	}
}

// WithGORMResolver replaces the default row-based lookup with a caller-
// supplied function. The function receives the *gorm.DB the adapter was
// constructed with and is responsible for returning a normalized
// SubscriptionClaim or (nil, nil) for "no record".
func WithGORMResolver(r Resolver) GORMOption {
	return func(g *GORM) {
		if r != nil {
			g.resolver = r
		}
	}
}

// WithGORMProvider overrides the Provider string stamped onto claims that
// the default resolver constructs. Default is "gorm" — services that
// already populate the row's provider column should stick with the default
// (the row value wins when present); the option exists for projections
// that don't store provider per-row.
func WithGORMProvider(name string) GORMOption {
	return func(g *GORM) {
		if name != "" {
			g.providerFallback = name
		}
	}
}

// GORM resolves SubscriptionClaim values from a relational projection
// owned by the consumer service. Implements application.SubscriptionService.
//
// The default resolver expects a table matching SubscriptionRecord. When
// accountID is non-empty the lookup requires an exact (agent_id,
// account_id) match — the agent-only row is NOT used as a fallback,
// because a paid personal-account subscription must not silently grant
// paid-tier access to a B2B account the same agent belongs to. When
// accountID is empty, the agent-only row (account_id = '' or IS NULL)
// is matched. Among matches, latest updated_at wins; ties break on the
// row's primary key id so output is deterministic. For schemas that
// don't fit that shape, supply WithGORMResolver.
type GORM struct {
	db               *gorm.DB
	table            string
	resolver         Resolver
	providerFallback string
}

// NewGORM returns a GORM adapter backed by db. The default lookup queries
// the "subscriptions" table; pass WithGORMTable or WithGORMResolver to
// adapt to a different schema.
func NewGORM(db *gorm.DB, opts ...GORMOption) *GORM {
	g := &GORM{
		db:               db,
		table:            gormDefaultTable,
		providerFallback: "gorm",
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

// GetSubscription resolves the claim for agentID (optionally scoped to
// accountID). Returns (nil, nil) for "no record"; an error for actual
// database failures, a nil database, or a returned claim whose status
// doesn't match the closed set of SubscriptionStatus values (which would
// otherwise silently propagate a misconfigured row or buggy resolver
// into the issued JWT).
func (g *GORM) GetSubscription(ctx context.Context, agentID, accountID string) (*auth.SubscriptionClaim, error) {
	if g.db == nil {
		return nil, errors.New("gorm: database is nil")
	}
	if agentID == "" {
		return nil, errors.New("gorm: agentID must not be empty")
	}
	var (
		claim *auth.SubscriptionClaim
		err   error
	)
	if g.resolver != nil {
		claim, err = g.resolver(ctx, g.db, agentID, accountID)
	} else {
		claim, err = g.defaultLookup(ctx, agentID, accountID)
	}
	if err != nil || claim == nil {
		return claim, err
	}
	if !claim.Status.Valid() {
		return nil, fmt.Errorf("gorm: invalid subscription status %q for agent %s", claim.Status, agentID)
	}
	return claim, nil
}

func (g *GORM) defaultLookup(ctx context.Context, agentID, accountID string) (*auth.SubscriptionClaim, error) {
	var rec SubscriptionRecord
	base := g.db.WithContext(ctx).Table(g.table).Where("agent_id = ?", agentID)

	if accountID != "" {
		err := base.Where("account_id = ?", accountID).
			Order("updated_at DESC, id DESC").
			Limit(1).
			Take(&rec).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("gorm: query account-scoped subscription: %w", err)
		}
		return g.toClaim(rec), nil
	}

	err := base.Where("account_id = ? OR account_id IS NULL", "").
		Order("updated_at DESC, id DESC").
		Limit(1).
		Take(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gorm: query subscription: %w", err)
	}
	return g.toClaim(rec), nil
}

func (g *GORM) toClaim(rec SubscriptionRecord) *auth.SubscriptionClaim {
	provider := rec.Provider
	if provider == "" {
		provider = g.providerFallback
	}
	claim := &auth.SubscriptionClaim{
		Status:   auth.SubscriptionStatus(rec.Status),
		Plan:     rec.Plan,
		Provider: provider,
	}
	if rec.ExpiresAt != nil {
		claim.ExpiresAt = rec.ExpiresAt.UTC()
	}
	return claim
}
