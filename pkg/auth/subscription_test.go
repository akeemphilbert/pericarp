package auth

import (
	"testing"
	"time"
)

func TestSubscriptionClaim_IsActive(t *testing.T) {
	t.Parallel()

	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-24 * time.Hour)

	tests := []struct {
		name   string
		claim  *SubscriptionClaim
		active bool
	}{
		{name: "nil claim", claim: nil, active: false},
		{name: "active no expiry", claim: &SubscriptionClaim{Status: SubscriptionStatusActive}, active: true},
		{name: "active expires future", claim: &SubscriptionClaim{Status: SubscriptionStatusActive, ExpiresAt: future}, active: true},
		{name: "active expires past", claim: &SubscriptionClaim{Status: SubscriptionStatusActive, ExpiresAt: past}, active: false},
		{name: "trialing no expiry", claim: &SubscriptionClaim{Status: SubscriptionStatusTrialing}, active: true},
		{name: "trialing expires future", claim: &SubscriptionClaim{Status: SubscriptionStatusTrialing, ExpiresAt: future}, active: true},
		{name: "trialing expires past", claim: &SubscriptionClaim{Status: SubscriptionStatusTrialing, ExpiresAt: past}, active: false},
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

func TestSubscriptionStatus_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status SubscriptionStatus
		valid  bool
	}{
		{SubscriptionStatusActive, true},
		{SubscriptionStatusTrialing, true},
		{SubscriptionStatusPastDue, true},
		{SubscriptionStatusCancelled, true},
		{SubscriptionStatusInactive, true},
		{"ACTIVE", false}, // wrong case
		{"trial", false},  // truncated
		{"", false},
		{"frobnicated", false},
	}

	for _, tc := range tests {
		t.Run(string(tc.status), func(t *testing.T) {
			t.Parallel()
			if got := tc.status.Valid(); got != tc.valid {
				t.Errorf("Valid(%q) = %v, want %v", tc.status, got, tc.valid)
			}
		})
	}
}
