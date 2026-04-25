package auth

import "time"

// SubscriptionStatus enumerates the lifecycle states a subscription can be in.
// Provider-specific states are normalized into one of these values so consumer
// services can gate access uniformly regardless of billing backend.
type SubscriptionStatus string

const (
	// SubscriptionStatusActive — paid and current. Grant paid-tier access.
	SubscriptionStatusActive SubscriptionStatus = "active"
	// SubscriptionStatusTrialing — in a trial period. Grant paid-tier access.
	SubscriptionStatusTrialing SubscriptionStatus = "trialing"
	// SubscriptionStatusPastDue — payment failed but provider has not yet
	// cancelled. Treated as inactive by IsActive; consumers may choose a
	// grace-period policy on top.
	SubscriptionStatusPastDue SubscriptionStatus = "past_due"
	// SubscriptionStatusCancelled — explicitly cancelled (no longer renewing).
	SubscriptionStatusCancelled SubscriptionStatus = "cancelled"
	// SubscriptionStatusInactive — no active subscription found.
	SubscriptionStatusInactive SubscriptionStatus = "inactive"
)

// SubscriptionClaim is the normalized subscription snapshot embedded in an
// identity token at issuance time. Consumers read it off the validated token
// instead of looking up billing state per request.
//
// Metadata carries provider-specific fields (e.g., Stripe metadata,
// RevenueCat entitlement keys). It is preserved verbatim so consumers can
// reach into provider-native data when the normalized fields are not enough.
type SubscriptionClaim struct {
	Status    SubscriptionStatus `json:"status"`
	Plan      string             `json:"plan,omitempty"`
	Provider  string             `json:"provider,omitempty"`
	ExpiresAt time.Time          `json:"expires_at,omitzero"`
	Metadata  map[string]any     `json:"metadata,omitempty"`
}

// IsActive reports whether the subscription should grant paid-tier access.
// A nil claim is inactive by definition (no subscription was looked up or
// the provider returned no record for the agent).
func (s *SubscriptionClaim) IsActive() bool {
	if s == nil {
		return false
	}
	switch s.Status {
	case SubscriptionStatusActive, SubscriptionStatusTrialing:
		return true
	}
	return false
}
