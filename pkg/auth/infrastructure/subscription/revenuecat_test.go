package subscription_test

import (
	"context"
	"io"
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

func TestRevenueCat_LifetimeRevoked_MarkedInactive(t *testing.T) {
	// Refunded / revoked lifetime: the entitlement is still in the
	// response while RevenueCat propagates the change, but the
	// subscription row carries unsubscribe_detected_at. The adapter
	// must treat this as inactive instead of silently reporting Active.
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	revoked := now.Add(-1 * time.Hour)
	body := `{
		"subscriber": {
			"entitlements": {"pro":{"expires_date":null,"product_identifier":"pro_lifetime"}},
			"subscriptions": {"pro_lifetime":{"expires_date":null,"unsubscribe_detected_at":"` + revoked.Format(time.RFC3339) + `"}}
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
		t.Errorf("Status = %q, want %q for revoked lifetime", claim.Status, auth.SubscriptionStatusInactive)
	}
	if claim.IsActive() {
		t.Error("IsActive() = true, want false for revoked lifetime")
	}
}

func TestRevenueCat_UnsubscribeButStillInPaidWindow_StaysActive(t *testing.T) {
	// User turned off auto-renew but the paid period hasn't lapsed yet.
	// They should keep paid-tier access until expires_date.
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	expires := now.Add(7 * 24 * time.Hour)
	turnedOff := now.Add(-3 * 24 * time.Hour)
	body := `{
		"subscriber": {
			"entitlements": {"pro":{"expires_date":"` + expires.Format(time.RFC3339) + `","product_identifier":"pro_monthly"}},
			"subscriptions": {"pro_monthly":{"expires_date":"` + expires.Format(time.RFC3339) + `","unsubscribe_detected_at":"` + turnedOff.Format(time.RFC3339) + `"}}
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
	if claim.Status != auth.SubscriptionStatusActive {
		t.Errorf("Status = %q, want %q while still in paid window", claim.Status, auth.SubscriptionStatusActive)
	}
}

func TestRevenueCat_BillingIssueOnUnmatchedSubscription_StillSurfaces(t *testing.T) {
	// Entitlement product_identifier doesn't match the subscription row's
	// key (multi-SKU offering, upgrade in flight). The fallback scan
	// must still surface the billing-issues signal rather than reporting
	// Active.
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	expires := now.Add(7 * 24 * time.Hour)
	billing := now.Add(-1 * time.Hour)
	body := `{
		"subscriber": {
			"entitlements": {"pro":{"expires_date":"` + expires.Format(time.RFC3339) + `","product_identifier":"pro_annual"}},
			"subscriptions": {"pro_monthly":{"expires_date":"` + expires.Format(time.RFC3339) + `","billing_issues_detected_at":"` + billing.Format(time.RFC3339) + `"}}
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
		t.Errorf("Status = %q, want %q (fallback scan should surface unmatched billing issue)", claim.Status, auth.SubscriptionStatusPastDue)
	}
}

func TestRevenueCat_TimeBoundedNoMatchingSubscription_Active(t *testing.T) {
	// Entitlement granted via promo code with no purchase row. Status
	// should default to Active rather than panic on the missing key.
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	expires := now.Add(30 * 24 * time.Hour)
	body := `{
		"subscriber": {
			"entitlements": {"pro":{"expires_date":"` + expires.Format(time.RFC3339) + `","product_identifier":"pro_monthly"}},
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
	if claim.Status != auth.SubscriptionStatusActive {
		t.Errorf("Status = %q, want Active for entitlement with no subscription row", claim.Status)
	}
	if _, ok := claim.Metadata["store"]; ok {
		t.Errorf("store metadata should be absent without matching subscription, got %v", claim.Metadata["store"])
	}
}

func TestRevenueCat_LifetimeAndBoundedCoexist_LifetimeWins(t *testing.T) {
	// Common shape after an upgrade: the bounded subscription is still
	// in the response while the lifetime entitlement supersedes it.
	// Lifetime must win regardless of map iteration order, so we run
	// the assertion several times to defeat luck.
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	expires := now.Add(60 * 24 * time.Hour)
	body := `{
		"subscriber": {
			"entitlements": {
				"pro_lifetime": {"expires_date":null,"product_identifier":"lifetime_sku"},
				"pro_monthly":  {"expires_date":"` + expires.Format(time.RFC3339) + `","product_identifier":"monthly_sku"}
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

	for i := 0; i < 10; i++ {
		claim, err := rc.GetSubscription(context.Background(), "agent-1", "")
		if err != nil {
			t.Fatalf("GetSubscription error: %v", err)
		}
		if claim.Plan != "pro_lifetime" {
			t.Fatalf("Plan = %q, want pro_lifetime (iteration %d)", claim.Plan, i)
		}
		if !claim.ExpiresAt.IsZero() {
			t.Fatalf("ExpiresAt = %v, want zero for lifetime (iteration %d)", claim.ExpiresAt, i)
		}
	}
}

func TestRevenueCat_TieBreakByName_PicksLexFirst(t *testing.T) {
	// Two entitlements with identical expiry — adapter breaks ties by
	// alphabetic name so the chosen Plan is stable across runs.
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	expires := now.Add(30 * 24 * time.Hour)
	body := `{
		"subscriber": {
			"entitlements": {
				"alpha": {"expires_date":"` + expires.Format(time.RFC3339) + `","product_identifier":"alpha_sku"},
				"beta":  {"expires_date":"` + expires.Format(time.RFC3339) + `","product_identifier":"beta_sku"}
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
	for i := 0; i < 10; i++ {
		claim, err := rc.GetSubscription(context.Background(), "agent-1", "")
		if err != nil {
			t.Fatalf("GetSubscription error: %v", err)
		}
		if claim.Plan != "alpha" {
			t.Fatalf("Plan = %q, want alpha (iteration %d)", claim.Plan, i)
		}
	}
}

func TestRevenueCat_MalformedJSON_ReturnsDecodeError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json {{`))
	}))
	defer srv.Close()

	rc := subscription.NewRevenueCat("k", subscription.WithRevenueCatBaseURL(srv.URL))
	claim, err := rc.GetSubscription(context.Background(), "agent-1", "")
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if claim != nil {
		t.Errorf("expected nil claim on decode error, got %+v", claim)
	}
	if !strings.Contains(err.Error(), "revenuecat") {
		t.Errorf("error = %v, want it to be namespaced revenuecat:", err)
	}
}

// recordingTransport satisfies http.RoundTripper and captures the last
// request observed, so we can assert that WithRevenueCatHTTPClient is
// actually wired through to the request path.
type recordingTransport struct {
	last *http.Request
	body string
}

func (t *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.last = req
	body := t.body
	if body == "" {
		body = `{"subscriber":{"entitlements":{},"subscriptions":{}}}`
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func TestRevenueCat_WithHTTPClient_UsesProvidedClient(t *testing.T) {
	t.Parallel()

	tr := &recordingTransport{}
	client := &http.Client{Transport: tr}

	rc := subscription.NewRevenueCat(
		"test-key",
		subscription.WithRevenueCatBaseURL("https://api.example.com/v1"),
		subscription.WithRevenueCatHTTPClient(client),
	)
	if _, err := rc.GetSubscription(context.Background(), "agent-1", ""); err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}

	if tr.last == nil {
		t.Fatal("custom transport was not used")
	}
	if got := tr.last.URL.String(); got != "https://api.example.com/v1/subscribers/agent-1" {
		t.Errorf("request URL = %q, want https://api.example.com/v1/subscribers/agent-1", got)
	}
	if got := tr.last.Header.Get("Authorization"); got != "Bearer test-key" {
		t.Errorf("Authorization = %q, want Bearer test-key", got)
	}
}

func TestRevenueCat_WithHTTPClient_NilNoOp(t *testing.T) {
	// Passing nil to the option must not clobber the default client.
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"subscriber":{"entitlements":{},"subscriptions":{}}}`))
	}))
	defer srv.Close()

	rc := subscription.NewRevenueCat("k",
		subscription.WithRevenueCatBaseURL(srv.URL),
		subscription.WithRevenueCatHTTPClient(nil),
	)
	if _, err := rc.GetSubscription(context.Background(), "agent-1", ""); err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
}

func TestRevenueCat_AgentIDIsPathEscaped(t *testing.T) {
	t.Parallel()

	tr := &recordingTransport{}
	client := &http.Client{Transport: tr}
	rc := subscription.NewRevenueCat(
		"k",
		subscription.WithRevenueCatBaseURL("https://api.example.com/v1"),
		subscription.WithRevenueCatHTTPClient(client),
	)
	// agent IDs containing slashes or spaces must not silently traverse
	// path segments or break URL parsing.
	if _, err := rc.GetSubscription(context.Background(), "weird/agent id", ""); err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if tr.last == nil {
		t.Fatal("transport was not used")
	}
	want := "https://api.example.com/v1/subscribers/weird%2Fagent%20id"
	if got := tr.last.URL.String(); got != want {
		t.Errorf("URL = %q, want %q", got, want)
	}
}
