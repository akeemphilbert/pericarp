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
// The default resolver expects a table matching SubscriptionRecord and
// picks, for the given (agentID, accountID), the row with the latest
// updated_at — preferring an exact account match when accountID is
// non-empty, falling back to the agent-only row otherwise. For schemas
// that don't fit that shape, supply WithGORMResolver.
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
// database failures or a nil database.
func (g *GORM) GetSubscription(ctx context.Context, agentID, accountID string) (*auth.SubscriptionClaim, error) {
	if g.db == nil {
		return nil, errors.New("gorm: database is nil")
	}
	if agentID == "" {
		return nil, errors.New("gorm: agentID must not be empty")
	}
	if g.resolver != nil {
		return g.resolver(ctx, g.db, agentID, accountID)
	}
	return g.defaultLookup(ctx, agentID, accountID)
}

func (g *GORM) defaultLookup(ctx context.Context, agentID, accountID string) (*auth.SubscriptionClaim, error) {
	var rec SubscriptionRecord
	q := g.db.WithContext(ctx).Table(g.table).Where("agent_id = ?", agentID)

	// Prefer an exact account match when one was requested. Fall back to
	// the latest agent-scoped row only if no account-scoped row exists,
	// so a paid personal-account subscription doesn't accidentally apply
	// to a B2B account the same agent belongs to.
	if accountID != "" {
		err := q.Where("account_id = ?", accountID).
			Order("updated_at DESC").
			Limit(1).
			Take(&rec).Error
		if err == nil {
			return g.toClaim(rec), nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("gorm: query account-scoped subscription: %w", err)
		}
	}

	err := g.db.WithContext(ctx).
		Table(g.table).
		Where("agent_id = ? AND (account_id = ? OR account_id IS NULL)", agentID, "").
		Order("updated_at DESC").
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
