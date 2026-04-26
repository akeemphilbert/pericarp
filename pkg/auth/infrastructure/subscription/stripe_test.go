package subscription_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/subscription"
)

var _ application.SubscriptionService = (*subscription.Stripe)(nil)

// stripeFixture builds a Stripe customer-search response body with the
// given subscriptions. Only fields the adapter consumes are populated.
func stripeFixture(subs ...string) string {
	if len(subs) == 0 {
		return `{"data":[]}`
	}
	return `{"data":[{"id":"cus_1","subscriptions":{"data":[` + strings.Join(subs, ",") + `]}}]}`
}

func stripeSub(id, status string, periodEnd int64, lookupKey string, cancelAtPeriodEnd bool) string {
	cape := "false"
	if cancelAtPeriodEnd {
		cape = "true"
	}
	return `{"id":"` + id + `","status":"` + status + `","current_period_end":` + strconv.FormatInt(periodEnd, 10) + `,"cancel_at_period_end":` + cape + `,"items":{"data":[{"price":{"lookup_key":"` + lookupKey + `"}}]}}`
}

func TestStripe_NoCustomerMatch_ReturnsNil(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim != nil {
		t.Errorf("expected nil claim, got %+v", claim)
	}
}

func TestStripe_CustomerWithNoSubscriptions_ReturnsNil(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"id":"cus_1","subscriptions":{"data":[]}}]}`))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim != nil {
		t.Errorf("expected nil claim, got %+v", claim)
	}
}

func TestStripe_ActiveSubscription(t *testing.T) {
	t.Parallel()

	periodEnd := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).Unix()
	body := stripeFixture(stripeSub("sub_1", "active", periodEnd, "pro_monthly", false))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify HTTP basic auth uses the API key.
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Basic ") {
			t.Errorf("Authorization scheme = %q, want Basic", authHeader)
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authHeader, "Basic "))
		if err != nil {
			t.Fatalf("could not decode basic auth: %v", err)
		}
		if string(decoded) != "sk_test:" {
			t.Errorf("basic auth = %q, want sk_test:", decoded)
		}
		// Verify the search query and expand parameters.
		q := r.URL.Query().Get("query")
		if !strings.Contains(q, "metadata['agent_id']:'agent-1'") {
			t.Errorf("query = %q, want metadata['agent_id']:'agent-1'", q)
		}
		if got := r.URL.Query()["expand[]"]; len(got) != 1 || got[0] != "data.subscriptions" {
			t.Errorf("expand = %v, want [data.subscriptions]", got)
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test",
		subscription.WithStripeBaseURL(srv.URL),
		subscription.WithStripeIncludeBillingIDs(true),
	)
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim == nil {
		t.Fatal("expected non-nil claim")
	}
	if claim.Status != auth.SubscriptionStatusActive {
		t.Errorf("Status = %q, want %q", claim.Status, auth.SubscriptionStatusActive)
	}
	if claim.Plan != "pro_monthly" {
		t.Errorf("Plan = %q, want %q", claim.Plan, "pro_monthly")
	}
	if claim.Provider != "stripe" {
		t.Errorf("Provider = %q, want %q", claim.Provider, "stripe")
	}
	if !claim.ExpiresAt.Equal(time.Unix(periodEnd, 0).UTC()) {
		t.Errorf("ExpiresAt = %v, want %v", claim.ExpiresAt, time.Unix(periodEnd, 0).UTC())
	}
	if got := claim.Metadata["subscription_id"]; got != "sub_1" {
		t.Errorf("Metadata[subscription_id] = %v, want sub_1", got)
	}
	if _, ok := claim.Metadata["cancel_at_period_end"]; ok {
		t.Errorf("cancel_at_period_end should be absent when false, got %v", claim.Metadata["cancel_at_period_end"])
	}
}

func TestStripe_TrialingSubscription(t *testing.T) {
	t.Parallel()

	periodEnd := time.Now().Add(7 * 24 * time.Hour).Unix()
	body := stripeFixture(stripeSub("sub_1", "trialing", periodEnd, "trial", false))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Status != auth.SubscriptionStatusTrialing {
		t.Errorf("Status = %q, want %q", claim.Status, auth.SubscriptionStatusTrialing)
	}
}

func TestStripe_PastDueSubscription(t *testing.T) {
	t.Parallel()

	periodEnd := time.Now().Add(7 * 24 * time.Hour).Unix()
	body := stripeFixture(stripeSub("sub_1", "past_due", periodEnd, "pro", false))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Status != auth.SubscriptionStatusPastDue {
		t.Errorf("Status = %q, want %q", claim.Status, auth.SubscriptionStatusPastDue)
	}
}

func TestStripe_CanceledSubscription(t *testing.T) {
	t.Parallel()

	body := stripeFixture(stripeSub("sub_1", "canceled", 0, "pro", false))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Status != auth.SubscriptionStatusCancelled {
		t.Errorf("Status = %q, want %q", claim.Status, auth.SubscriptionStatusCancelled)
	}
}

func TestStripe_IncompleteOrPaused_MapToInactive(t *testing.T) {
	t.Parallel()

	for _, status := range []string{"incomplete", "incomplete_expired", "unpaid", "paused"} {
		status := status
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			body := stripeFixture(stripeSub("sub_1", status, 0, "pro", false))
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
			claim, err := s.GetSubscription(context.Background(), "agent-1", "")
			if err != nil {
				t.Fatalf("GetSubscription error: %v", err)
			}
			if claim.Status != auth.SubscriptionStatusInactive {
				t.Errorf("Status = %q, want %q for stripe status %q", claim.Status, auth.SubscriptionStatusInactive, status)
			}
		})
	}
}

func TestStripe_PrefersActiveOverCancelled(t *testing.T) {
	t.Parallel()

	periodEnd := time.Now().Add(30 * 24 * time.Hour).Unix()
	body := stripeFixture(
		stripeSub("sub_old", "canceled", 0, "old_plan", false),
		stripeSub("sub_new", "active", periodEnd, "new_plan", false),
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Status != auth.SubscriptionStatusActive {
		t.Errorf("Status = %q, want active (preferred over canceled)", claim.Status)
	}
	if claim.Plan != "new_plan" {
		t.Errorf("Plan = %q, want new_plan", claim.Plan)
	}
}

func TestStripe_TwoActive_PicksLaterPeriodEnd(t *testing.T) {
	t.Parallel()

	earlier := time.Now().Add(7 * 24 * time.Hour).Unix()
	later := time.Now().Add(60 * 24 * time.Hour).Unix()
	body := stripeFixture(
		stripeSub("sub_a", "active", earlier, "early_plan", false),
		stripeSub("sub_b", "active", later, "late_plan", false),
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Plan != "late_plan" {
		t.Errorf("Plan = %q, want late_plan (later period_end)", claim.Plan)
	}
}

func TestStripe_CancelAtPeriodEnd_StaysActive_FlagsMetadata(t *testing.T) {
	t.Parallel()

	periodEnd := time.Now().Add(7 * 24 * time.Hour).Unix()
	body := stripeFixture(stripeSub("sub_1", "active", periodEnd, "pro", true))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Status != auth.SubscriptionStatusActive {
		t.Errorf("Status = %q, want active until period actually ends", claim.Status)
	}
	if got := claim.Metadata["cancel_at_period_end"]; got != true {
		t.Errorf("cancel_at_period_end metadata = %v, want true", got)
	}
}

func TestStripe_PlanFallback_NicknameThenProduct(t *testing.T) {
	t.Parallel()

	body := `{"data":[{"id":"cus_1","subscriptions":{"data":[{"id":"sub_1","status":"active","current_period_end":0,"items":{"data":[{"price":{"nickname":"Pro Plan","product":"prod_xyz"}}]}}]}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Plan != "Pro Plan" {
		t.Errorf("Plan = %q, want Pro Plan (nickname fallback)", claim.Plan)
	}

	body = `{"data":[{"id":"cus_1","subscriptions":{"data":[{"id":"sub_1","status":"active","items":{"data":[{"price":{"product":"prod_xyz"}}]}}]}}]}`
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv2.Close()
	s2 := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv2.URL))
	claim2, err := s2.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim2.Plan != "prod_xyz" {
		t.Errorf("Plan = %q, want prod_xyz (product fallback)", claim2.Plan)
	}
}

func TestStripe_CustomMetadataKey(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if !strings.Contains(q, "metadata['pericarp_agent']:'agent-1'") {
			t.Errorf("query = %q, want metadata['pericarp_agent']:'agent-1'", q)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test",
		subscription.WithStripeBaseURL(srv.URL),
		subscription.WithStripeAgentMetadataKey("pericarp_agent"),
	)
	if _, err := s.GetSubscription(context.Background(), "agent-1", ""); err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
}

func TestStripe_AgentIDQuoteEscaped(t *testing.T) {
	// A defensive escape against an apostrophe in the agent ID
	// breaking out of the search query string.
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, err := url.QueryUnescape(r.URL.Query().Get("query"))
		if err != nil {
			t.Fatalf("decode query: %v", err)
		}
		if !strings.Contains(q, `metadata['agent_id']:'agent\'1'`) {
			t.Errorf("query = %q, want backslash-escaped apostrophe", q)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	if _, err := s.GetSubscription(context.Background(), "agent'1", ""); err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
}

func TestStripe_NotFound_ReturnsNilNoError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim != nil {
		t.Errorf("expected nil claim for 404, got %+v", claim)
	}
}

func TestStripe_NonOKStatus_ReturnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid query"}}`))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	_, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error = %v, want it to mention 400", err)
	}
}

func TestStripe_MalformedJSON_ReturnsDecodeError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json {{`))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err == nil {
		t.Fatal("expected decode error")
	}
	if claim != nil {
		t.Errorf("expected nil claim, got %+v", claim)
	}
	if !strings.Contains(err.Error(), "stripe") {
		t.Errorf("error = %v, want it namespaced stripe:", err)
	}
}

func TestStripe_MissingAPIKey_Errors(t *testing.T) {
	t.Parallel()

	s := subscription.NewStripe("")
	if _, err := s.GetSubscription(context.Background(), "agent-1", ""); err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestStripe_EmptyAgentID_Errors(t *testing.T) {
	t.Parallel()

	s := subscription.NewStripe("sk_test")
	if _, err := s.GetSubscription(context.Background(), "", ""); err == nil {
		t.Fatal("expected error for empty agent ID")
	}
}

func TestStripe_CancelledContext_Errors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.GetSubscription(ctx, "agent-1", ""); err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestStripe_WithHTTPClient_UsesProvidedClient(t *testing.T) {
	t.Parallel()

	tr := &recordingTransport{body: `{"data":[]}`}
	client := &http.Client{Transport: tr}

	s := subscription.NewStripe("sk_test",
		subscription.WithStripeBaseURL("https://api.example.com/v1"),
		subscription.WithStripeHTTPClient(client),
	)
	if _, err := s.GetSubscription(context.Background(), "agent-1", ""); err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if tr.last == nil {
		t.Fatal("custom transport not used")
	}
	if !strings.HasPrefix(tr.last.URL.String(), "https://api.example.com/v1/customers/search") {
		t.Errorf("URL = %q, want prefix https://api.example.com/v1/customers/search", tr.last.URL.String())
	}
}

func TestStripe_CanceledButStillInPaidWindow_StaysActive(t *testing.T) {
	// Stripe flips status to "canceled" the moment the merchant cancels
	// even though current_period_end is still in the future and the
	// customer is entitled to access through that date. Adapter must
	// keep IsActive() = true and surface the cancellation in metadata.
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	periodEnd := now.Add(7 * 24 * time.Hour).Unix()
	body := stripeFixture(stripeSub("sub_1", "canceled", periodEnd, "pro", false))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test",
		subscription.WithStripeBaseURL(srv.URL),
		subscription.WithStripeNow(func() time.Time { return now }),
	)
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Status != auth.SubscriptionStatusActive {
		t.Errorf("Status = %q, want Active while still in paid window", claim.Status)
	}
	if !claim.IsActive() {
		t.Error("IsActive() = false, want true")
	}
	if got := claim.Metadata["cancel_at_period_end"]; got != true {
		t.Errorf("cancel_at_period_end metadata = %v, want true", got)
	}
}

func TestStripe_CanceledExpired_StaysCancelled(t *testing.T) {
	// Same status string, but the paid window has lapsed — adapter must
	// fall through to Cancelled.
	t.Parallel()

	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	expired := now.Add(-7 * 24 * time.Hour).Unix()
	body := stripeFixture(stripeSub("sub_1", "canceled", expired, "pro", false))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test",
		subscription.WithStripeBaseURL(srv.URL),
		subscription.WithStripeNow(func() time.Time { return now }),
	)
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Status != auth.SubscriptionStatusCancelled {
		t.Errorf("Status = %q, want Cancelled after period lapsed", claim.Status)
	}
}

func TestStripe_CanceledZeroPeriodEnd_StaysCancelled(t *testing.T) {
	// current_period_end == 0 (no expiry stamped) collapses to Cancelled
	// — there's no future window to honor. ExpiresAt should also stay
	// zero so IsActive doesn't accidentally flip on a missing expiry.
	t.Parallel()

	body := stripeFixture(stripeSub("sub_1", "canceled", 0, "pro", false))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Status != auth.SubscriptionStatusCancelled {
		t.Errorf("Status = %q, want Cancelled", claim.Status)
	}
	if !claim.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero when current_period_end == 0", claim.ExpiresAt)
	}
}

func TestStripe_TrialingBeatsPastDue(t *testing.T) {
	t.Parallel()

	periodEnd := time.Now().Add(7 * 24 * time.Hour).Unix()
	body := stripeFixture(
		stripeSub("sub_pd", "past_due", periodEnd, "pd_plan", false),
		stripeSub("sub_tr", "trialing", periodEnd, "tr_plan", false),
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Plan != "tr_plan" {
		t.Errorf("Plan = %q, want tr_plan (trialing beats past_due)", claim.Plan)
	}
}

func TestStripe_TieBreakBySubscriptionID(t *testing.T) {
	t.Parallel()

	periodEnd := time.Now().Add(30 * 24 * time.Hour).Unix()
	body := stripeFixture(
		stripeSub("sub_b", "active", periodEnd, "b_plan", false),
		stripeSub("sub_a", "active", periodEnd, "a_plan", false),
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	for i := 0; i < 5; i++ {
		claim, err := s.GetSubscription(context.Background(), "agent-1", "")
		if err != nil {
			t.Fatalf("GetSubscription error: %v", err)
		}
		if claim.Plan != "a_plan" {
			t.Fatalf("Plan = %q, want a_plan (lex-first ID), iteration %d", claim.Plan, i)
		}
	}
}

func TestStripe_MultipleCustomers_PicksBestAcrossAll(t *testing.T) {
	// Outer loop over search.Data must be exercised. The active
	// subscription is on cus_2; cus_1 only has a canceled one. The
	// adapter walks both and picks the active.
	t.Parallel()

	periodEnd := time.Now().Add(30 * 24 * time.Hour).Unix()
	body := `{"data":[
		{"id":"cus_1","subscriptions":{"data":[` + stripeSub("sub_old", "canceled", 0, "old_plan", false) + `]}},
		{"id":"cus_2","subscriptions":{"data":[` + stripeSub("sub_new", "active", periodEnd, "new_plan", false) + `]}}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test",
		subscription.WithStripeBaseURL(srv.URL),
		subscription.WithStripeIncludeBillingIDs(true),
	)
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Plan != "new_plan" {
		t.Errorf("Plan = %q, want new_plan (active wins across customers)", claim.Plan)
	}
	if got := claim.Metadata["customer_id"]; got != "cus_2" {
		t.Errorf("Metadata[customer_id] = %v, want cus_2", got)
	}
	if got := claim.Metadata["customer_match_count"]; got != 2 {
		t.Errorf("Metadata[customer_match_count] = %v, want 2 (split-brain marker)", got)
	}
}

func TestStripe_BillingIDsOmittedByDefault(t *testing.T) {
	// customer_id and subscription_id are sensitive billing identifiers.
	// They land in JWTs and downstream service logs if embedded, so the
	// adapter omits them unless the caller explicitly opts in via
	// WithStripeIncludeBillingIDs. customer_match_count is unaffected
	// because it's a count, not an identifier.
	t.Parallel()

	periodEnd := time.Now().Add(30 * 24 * time.Hour).Unix()
	body := `{"data":[
		{"id":"cus_1","subscriptions":{"data":[` + stripeSub("sub_old", "canceled", 0, "old_plan", false) + `]}},
		{"id":"cus_2","subscriptions":{"data":[` + stripeSub("sub_new", "active", periodEnd, "new_plan", false) + `]}}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if _, ok := claim.Metadata["customer_id"]; ok {
		t.Errorf("Metadata[customer_id] should be absent by default, got %v", claim.Metadata["customer_id"])
	}
	if _, ok := claim.Metadata["subscription_id"]; ok {
		t.Errorf("Metadata[subscription_id] should be absent by default, got %v", claim.Metadata["subscription_id"])
	}
	if got := claim.Metadata["customer_match_count"]; got != 2 {
		t.Errorf("Metadata[customer_match_count] = %v, want 2 (still surfaces split-brain without leaking IDs)", got)
	}
}

func TestStripe_EmptyItems_PlanIsEmpty(t *testing.T) {
	t.Parallel()

	body := `{"data":[{"id":"cus_1","subscriptions":{"data":[{"id":"sub_1","status":"active","current_period_end":0,"items":{"data":[]}}]}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test", subscription.WithStripeBaseURL(srv.URL))
	claim, err := s.GetSubscription(context.Background(), "agent-1", "")
	if err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
	if claim.Plan != "" {
		t.Errorf("Plan = %q, want empty for subscription with no items", claim.Plan)
	}
	if claim.Status != auth.SubscriptionStatusActive {
		t.Errorf("Status = %q, want Active", claim.Status)
	}
}

func TestStripe_WithHTTPClient_NilNoOp(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	s := subscription.NewStripe("sk_test",
		subscription.WithStripeBaseURL(srv.URL),
		subscription.WithStripeHTTPClient(nil),
	)
	if _, err := s.GetSubscription(context.Background(), "agent-1", ""); err != nil {
		t.Fatalf("GetSubscription error: %v", err)
	}
}

