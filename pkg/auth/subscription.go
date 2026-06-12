package auth

import "time"

// SubscriptionStatus enumerates the lifecycle states a subscription can be
// in. Provider-specific states are normalized into one of these values so
// consumers can gate access uniformly regardless of billing backend.
type SubscriptionStatus string

const (
	SubscriptionStatusActive   SubscriptionStatus = "active"
	SubscriptionStatusTrialing SubscriptionStatus = "trialing"
	// PastDue is split out from Cancelled because billing providers commonly
	// flag a failed payment without ending the subscription. Treated as
	// inactive by IsActive; consumers may layer a grace-period policy on top.
	SubscriptionStatusPastDue   SubscriptionStatus = "past_due"
	SubscriptionStatusCancelled SubscriptionStatus = "cancelled"
	SubscriptionStatusInactive  SubscriptionStatus = "inactive"
)

// Valid reports whether s is one of the defined SubscriptionStatus
// constants. Useful in adapter-side tests to catch provider-string drift
// (e.g. "ACTIVE" vs "active") before a malformed status reaches token
// issuance and silently downgrades a paying customer.
func (s SubscriptionStatus) Valid() bool {
	switch s {
	case SubscriptionStatusActive,
		SubscriptionStatusTrialing,
		SubscriptionStatusPastDue,
		SubscriptionStatusCancelled,
		SubscriptionStatusInactive:
		return true
	}
	return false
}

// SubscriptionClaim is the normalized subscription snapshot embedded in an
// identity token at issuance time. Consumers read it off the validated
// token instead of looking up billing state per request. Metadata is the
// open extension space for provider-specific fields (Stripe metadata,
// RevenueCat entitlements); the four normalized fields above it are the
// minimal common shape every consumer can rely on.
type SubscriptionClaim struct {
	Status   SubscriptionStatus `json:"status"`
	Plan     string             `json:"plan,omitempty"`
	Provider string             `json:"provider,omitempty"`
	// ExpiresAt uses omitzero (Go 1.24+) because time.Time's zero value is
	// not the JSON zero value — omitempty would emit "0001-01-01T00:00:00Z"
	// for unset times instead of dropping the field.
	ExpiresAt time.Time      `json:"expires_at,omitzero"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// IsActive reports whether the subscription should grant paid-tier access
// right now. It is IsActiveAt evaluated at time.Now(); see IsActiveAt for
// the actual rules.
func (s *SubscriptionClaim) IsActive() bool {
	return s.IsActiveAt(time.Now())
}

// IsActiveAt reports whether the subscription should grant paid-tier access
// at the given instant. Returns false for a nil receiver, for any
// non-active/trialing status, and for an explicit ExpiresAt strictly before
// now — that last guard means a stale snapshot held across an
// account-switch (which preserves the claim verbatim) cannot grant access
// beyond the provider-attested expiry. A zero ExpiresAt is treated as "no
// expiry expressed" and ignored. Tests and any caller with an injected
// clock should use this instead of IsActive so the result does not depend
// on the wall clock.
func (s *SubscriptionClaim) IsActiveAt(now time.Time) bool {
	if s == nil {
		return false
	}
	if !s.ExpiresAt.IsZero() && now.After(s.ExpiresAt) {
		return false
	}
	switch s.Status {
	case SubscriptionStatusActive, SubscriptionStatusTrialing:
		return true
	}
	return false
}
