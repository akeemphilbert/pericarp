package auth

import "testing"

func TestSubscriptionClaim_IsActive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		claim  *SubscriptionClaim
		active bool
	}{
		{name: "nil claim", claim: nil, active: false},
		{name: "active", claim: &SubscriptionClaim{Status: SubscriptionStatusActive}, active: true},
		{name: "trialing", claim: &SubscriptionClaim{Status: SubscriptionStatusTrialing}, active: true},
		{name: "past_due", claim: &SubscriptionClaim{Status: SubscriptionStatusPastDue}, active: false},
		{name: "cancelled", claim: &SubscriptionClaim{Status: SubscriptionStatusCancelled}, active: false},
		{name: "inactive", claim: &SubscriptionClaim{Status: SubscriptionStatusInactive}, active: false},
		{name: "unknown status", claim: &SubscriptionClaim{Status: "frobnicated"}, active: false},
		{name: "empty status", claim: &SubscriptionClaim{}, active: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.claim.IsActive(); got != tc.active {
				t.Errorf("IsActive() = %v, want %v", got, tc.active)
			}
		})
	}
}
