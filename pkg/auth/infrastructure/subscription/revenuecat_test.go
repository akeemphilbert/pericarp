package subscription_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/subscription"
)

// Adapter must satisfy the SubscriptionService interface.
var _ application.SubscriptionService = (*subscription.RevenueCat)(nil)

func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

func TestRevenueCat_NoEntitlements_ReturnsNil(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", got)
		}
		if !strings.HasSuffix(r.URL.Path, "/subscribers/agent-1") {
			t.Errorf("path = %q, want suffix /subscribers/agent-1", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"subscriber":{"entitlements":{},"subscriptions":{}}}`))
	}))
	defer srv.Close()

	rc := subscription.NewRevenueCat(
		"test-key",
		subscription.WithRevenueCatBaseURL(srv.URL),
	)

	claim, err := rc.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim != nil {
		t.Fatalf("expected nil claim for empty entitlements, got %+v", claim)
	}
}

func TestRevenueCat_ActiveEntitlement(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	expires := now.Add(30 * 24 * time.Hour)
	body := `{
		"subscriber": {
			"entitlements": {
				"pro": {
					"expires_date": "` + expires.Format(time.RFC3339) + `",
					"product_identifier": "pro_monthly"
				}
			},
			"subscriptions": {
				"pro_monthly": {
					"expires_date": "` + expires.Format(time.RFC3339) + `",
					"period_type": "normal",
					"store": "app_store"
				}
			}
		}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	rc := subscription.NewRevenueCat("k",
		subscription.WithRevenueCatBaseURL(srv.URL),
		subscription.WithRevenueCatNow(fixedNow(now)),
	)

	claim, err := rc.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim == nil {
		t.Fatal("expected non-nil claim")
	}
	if claim.Status != auth.SubscriptionStatusActive {
		t.Errorf("Status = %q, want %q", claim.Status, auth.SubscriptionStatusActive)
	}
	if claim.Plan != "pro" {
		t.Errorf("Plan = %q, want %q", claim.Plan, "pro")
	}
	if claim.Provider != "revenuecat" {
		t.Errorf("Provider = %q, want %q", claim.Provider, "revenuecat")
	}
	if !claim.ExpiresAt.Equal(expires) {
		t.Errorf("ExpiresAt = %v, want %v", claim.ExpiresAt, expires)
	}
	if got := claim.Metadata["product_identifier"]; got != "pro_monthly" {
		t.Errorf("Metadata[product_identifier] = %v, want pro_monthly", got)
	}
	if got := claim.Metadata["store"]; got != "app_store" {
		t.Errorf("Metadata[store] = %v, want app_store", got)
	}
	if !claim.IsActive() {
		t.Error("IsActive() = false, want true")
	}
}

func TestRevenueCat_TrialPeriod_MarkedTrialing(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	expires := now.Add(7 * 24 * time.Hour)
	body := `{
		"subscriber": {
			"entitlements": {"pro":{"expires_date":"` + expires.Format(time.RFC3339) + `","product_identifier":"pro_monthly"}},
			"subscriptions": {"pro_monthly":{"expires_date":"` + expires.Format(time.RFC3339) + `","period_type":"trial"}}
		}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	rc := subscription.NewRevenueCat("k",
		subscription.WithRevenueCatBaseURL(srv.URL),
		subscription.WithRevenueCatNow(fixedNow(now)),
	)
	claim, err := rc.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Status != auth.SubscriptionStatusTrialing {
		t.Errorf("Status = %q, want %q", claim.Status, auth.SubscriptionStatusTrialing)
	}
}

func TestRevenueCat_BillingIssue_MarkedPastDue(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	expires := now.Add(7 * 24 * time.Hour)
	billing := now.Add(-1 * time.Hour)
	body := `{
		"subscriber": {
			"entitlements": {"pro":{"expires_date":"` + expires.Format(time.RFC3339) + `","product_identifier":"pro_monthly"}},
			"subscriptions": {"pro_monthly":{"expires_date":"` + expires.Format(time.RFC3339) + `","period_type":"normal","billing_issues_detected_at":"` + billing.Format(time.RFC3339) + `"}}
		}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	rc := subscription.NewRevenueCat("k",
		subscription.WithRevenueCatBaseURL(srv.URL),
		subscription.WithRevenueCatNow(fixedNow(now)),
	)
	claim, err := rc.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Status != auth.SubscriptionStatusPastDue {
		t.Errorf("Status = %q, want %q", claim.Status, auth.SubscriptionStatusPastDue)
	}
}

func TestRevenueCat_ExpiredEntitlement_MarkedInactive(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-7 * 24 * time.Hour)
	body := `{
		"subscriber": {
			"entitlements": {"pro":{"expires_date":"` + expired.Format(time.RFC3339) + `","product_identifier":"pro_monthly"}},
			"subscriptions": {"pro_monthly":{"expires_date":"` + expired.Format(time.RFC3339) + `","period_type":"normal"}}
		}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	rc := subscription.NewRevenueCat("k",
		subscription.WithRevenueCatBaseURL(srv.URL),
		subscription.WithRevenueCatNow(fixedNow(now)),
	)
	claim, err := rc.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Status != auth.SubscriptionStatusInactive {
		t.Errorf("Status = %q, want %q", claim.Status, auth.SubscriptionStatusInactive)
	}
	if claim.IsActive() {
		t.Error("IsActive() = true, want false for expired entitlement")
	}
}

func TestRevenueCat_LifetimeEntitlement_NoExpiresAt(t *testing.T) {
	t.Parallel()

	body := `{"subscriber":{"entitlements":{"pro":{"expires_date":null,"product_identifier":"pro_lifetime"}},"subscriptions":{}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	rc := subscription.NewRevenueCat("k", subscription.WithRevenueCatBaseURL(srv.URL))
	claim, err := rc.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if !claim.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero for lifetime entitlement", claim.ExpiresAt)
	}
	if claim.Status != auth.SubscriptionStatusActive {
		t.Errorf("Status = %q, want %q", claim.Status, auth.SubscriptionStatusActive)
	}
	if !claim.IsActive() {
		t.Error("IsActive() = false, want true for lifetime entitlement")
	}
}

func TestRevenueCat_MultipleEntitlements_PicksLatestExpiry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	earlier := now.Add(7 * 24 * time.Hour)
	later := now.Add(60 * 24 * time.Hour)
	body := `{
		"subscriber": {
			"entitlements": {
				"basic": {"expires_date":"` + earlier.Format(time.RFC3339) + `","product_identifier":"basic_monthly"},
				"pro":   {"expires_date":"` + later.Format(time.RFC3339) + `","product_identifier":"pro_monthly"}
			},
			"subscriptions": {}
		}
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	rc := subscription.NewRevenueCat("k",
		subscription.WithRevenueCatBaseURL(srv.URL),
		subscription.WithRevenueCatNow(fixedNow(now)),
	)
	claim, err := rc.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Plan != "pro" {
		t.Errorf("Plan = %q, want pro (latest-expiring)", claim.Plan)
	}
	if !claim.ExpiresAt.Equal(later) {
		t.Errorf("ExpiresAt = %v, want %v", claim.ExpiresAt, later)
	}
}

func TestRevenueCat_NotFound_ReturnsNilNoError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	rc := subscription.NewRevenueCat("k", subscription.WithRevenueCatBaseURL(srv.URL))
	claim, err := rc.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim != nil {
		t.Errorf("expected nil claim for 404, got %+v", claim)
	}
}

func TestRevenueCat_NonOKStatus_ReturnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}))
	defer srv.Close()

	rc := subscription.NewRevenueCat("k", subscription.WithRevenueCatBaseURL(srv.URL))
	claim, err := rc.GetSubscription(context.Background(), "agent-1", "")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if claim != nil {
		t.Errorf("expected nil claim on error, got %+v", claim)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %v, want it to mention 500", err)
	}
}

func TestRevenueCat_MissingAPIKey_Errors(t *testing.T) {
	t.Parallel()

	rc := subscription.NewRevenueCat("")
	if _, err := rc.GetSubscription(context.Background(), "agent-1", ""); err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestRevenueCat_EmptyAgentID_Errors(t *testing.T) {
	t.Parallel()

	rc := subscription.NewRevenueCat("k")
	if _, err := rc.GetSubscription(context.Background(), "", ""); err == nil {
		t.Fatal("expected error for empty agent ID")
	}
}

func TestRevenueCat_CancelledContext_Errors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// never reached
	}))
	defer srv.Close()

	rc := subscription.NewRevenueCat("k", subscription.WithRevenueCatBaseURL(srv.URL))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := rc.GetSubscription(ctx, "agent-1", ""); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
