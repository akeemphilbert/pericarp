package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/golang-jwt/jwt/v5"
)

// Sentinel errors for JWT operations.
var (
	ErrTokenInvalid  = errors.New("authentication: token is invalid")
	ErrTokenExpired  = errors.New("authentication: token has expired")
	ErrSigningFailed = errors.New("authentication: failed to sign token")
	ErrNoSigningKey  = errors.New("authentication: no signing key configured")
	// ErrReservedClaim is returned when extras passed to IssueToken (or
	// carried on PericarpClaims at marshal time) contain keys that
	// collide with reserved JWT or pericarp core claims. All offending
	// keys are sorted and listed in the wrapped message so callers get a
	// deterministic, single-pass diagnosis rather than a flaky
	// one-key-at-a-time error driven by Go's map iteration order.
	ErrReservedClaim = errors.New("authentication: extras contain reserved claim names")
)

// reservedClaimNames is the set of top-level JWT claim names that
// pericarp owns. Extras passed to IssueToken cannot overwrite these,
// and ValidateToken excludes them from PericarpClaims.Extras on parse.
var reservedClaimNames = map[string]struct{}{
	// Standard JWT registered claims.
	"iss": {},
	"sub": {},
	"aud": {},
	"exp": {},
	"nbf": {},
	"iat": {},
	"jti": {},
	// Pericarp core claims.
	"agent_id":          {},
	"account_ids":       {},
	"active_account_id": {},
	"subscription":      {},
}

// ReservedClaimNames returns a fresh copy of the reserved JWT claim names
// that an extras map cannot overwrite. Useful for tests, validation
// helpers, or downstream JWTService implementations that want to mirror
// the protection.
func ReservedClaimNames() []string {
	names := make([]string, 0, len(reservedClaimNames))
	for k := range reservedClaimNames {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// IsReservedClaim reports whether name is a reserved top-level JWT claim
// owned by pericarp.
func IsReservedClaim(name string) bool {
	_, ok := reservedClaimNames[name]
	return ok
}

// ValidateExtras returns ErrReservedClaim listing every reserved key in
// extras (sorted), or nil when none collide. nil/empty extras are valid.
// JWTService implementations should call this before signing so that
// developers get a typed, single-shot diagnosis rather than fixing one
// reserved-key collision per deploy cycle.
func ValidateExtras(extras map[string]any) error {
	if len(extras) == 0 {
		return nil
	}
	var bad []string
	for k := range extras {
		if IsReservedClaim(k) {
			bad = append(bad, k)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	sort.Strings(bad)
	return fmt.Errorf("%w: %s", ErrReservedClaim, strings.Join(bad, ", "))
}

// PericarpClaims contains the JWT claims issued by the auth system.
// AgentID mirrors RegisteredClaims.Subject for convenient access without
// parsing the standard "sub" field. Subscription is set by
// AuthenticationService.IssueIdentityToken when a SubscriptionService is
// configured; the omitempty tag keeps the claim absent (rather than null)
// in opaque-session-only deployments.
//
// Extras carries app-specific claims attached by a ClaimsEnricher. They
// are flattened to top-level JWT claims on marshal and re-collected on
// unmarshal. Reserved claim names (see ReservedClaimNames) cannot reach
// the wire from Extras: MarshalJSON returns ErrReservedClaim if any are
// present, and UnmarshalJSON excludes them when parsing externally
// minted tokens. Numeric extras decode as float64 per encoding/json
// defaults — int64-precision values should be passed as strings.
type PericarpClaims struct {
	jwt.RegisteredClaims
	AgentID         string                  `json:"agent_id"`
	AccountIDs      []string                `json:"account_ids"`
	ActiveAccountID string                  `json:"active_account_id,omitempty"`
	Subscription    *auth.SubscriptionClaim `json:"subscription,omitempty"`
	Extras          map[string]any          `json:"-"`
}

// claimsAlias mirrors PericarpClaims for the marshal/unmarshal paths.
// Aliasing instead of duplicating the field list eliminates drift when
// new core claims are added — but the alias type MUST stay private and
// MUST NOT regain a custom MarshalJSON/UnmarshalJSON, otherwise
// PericarpClaims.MarshalJSON's call to json.Marshal would recurse and
// stack-overflow.
type claimsAlias PericarpClaims

// MarshalJSON flattens Extras into the top-level JWT claims object
// alongside the core claims. If Extras contains any reserved claim
// name (defense in depth — IssueToken's ValidateExtras call is the
// developer-facing gate), MarshalJSON returns ErrReservedClaim instead
// of silently skipping the offending keys: a missing claim downstream
// is far harder to diagnose than a refused token.
func (c PericarpClaims) MarshalJSON() ([]byte, error) {
	if err := ValidateExtras(c.Extras); err != nil {
		return nil, err
	}
	alias := claimsAlias(c)
	alias.Extras = nil
	base, err := json.Marshal(alias)
	if err != nil {
		return nil, err
	}
	if len(c.Extras) == 0 {
		return base, nil
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(base, &merged); err != nil {
		return nil, err
	}
	for k, v := range c.Extras {
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal extras[%q]: %w", k, err)
		}
		merged[k] = raw
	}
	return json.Marshal(merged)
}

// UnmarshalJSON populates the core claims and collects every other
// top-level key into Extras. Reserved claim names are excluded from
// Extras even when an externally minted token places them as siblings
// of the core fields — silent exclusion (rather than error) keeps
// validation tolerant of forged tokens that try to smuggle reserved
// names into Extras to bypass authorization checks reading the map.
func (c *PericarpClaims) UnmarshalJSON(data []byte) error {
	// Decode into a fresh alias and full-assign the receiver. This
	// resets every field (so a re-used *PericarpClaims cannot retain
	// residual values from a prior call) before we collect Extras.
	var alias claimsAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*c = PericarpClaims(alias)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	var extras map[string]any
	for k, v := range raw {
		if IsReservedClaim(k) {
			continue
		}
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			return fmt.Errorf("unmarshal extras[%q]: %w", k, err)
		}
		if extras == nil {
			extras = make(map[string]any, len(raw))
		}
		extras[k] = val
	}
	c.Extras = extras
	return nil
}

// JWTService defines the interface for issuing and validating JWTs.
//
// Implementations MUST reject extras containing reserved claim names
// (see ReservedClaimNames / ValidateExtras) by returning a wrapped
// ErrReservedClaim. This keeps the contract uniform across alternative
// signers so consumers can rely on the protection regardless of which
// JWTService is wired.
type JWTService interface {
	// IssueToken creates a signed JWT for the given agent and accounts.
	// A non-nil subscription is embedded as the "subscription" claim;
	// nil omits the claim. extras adds app-specific top-level claims;
	// nil/empty omits any extras. Reserved claim names in extras must
	// be rejected with a wrapped ErrReservedClaim.
	IssueToken(ctx context.Context, agent *entities.Agent, accounts []*entities.Account, activeAccountID string, subscription *auth.SubscriptionClaim, extras map[string]any) (string, error)

	// ValidateToken parses and validates a JWT string, returning the claims.
	// Non-reserved top-level claims are exposed via PericarpClaims.Extras.
	ValidateToken(ctx context.Context, tokenString string) (*PericarpClaims, error)
}
