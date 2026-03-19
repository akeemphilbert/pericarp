package main

import (
	"context"
	"errors"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	authcasbin "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/casbin"
	"github.com/casbin/casbin/v3/model"
)

// noopTestAdapter implements persist.Adapter for in-memory Casbin testing.
type noopTestAdapter struct{}

func (a *noopTestAdapter) LoadPolicy(model.Model) error                              { return nil }
func (a *noopTestAdapter) SavePolicy(model.Model) error                              { return nil }
func (a *noopTestAdapter) AddPolicy(string, string, []string) error                  { return nil }
func (a *noopTestAdapter) RemovePolicy(string, string, []string) error               { return nil }
func (a *noopTestAdapter) RemoveFilteredPolicy(string, string, int, ...string) error { return nil }

func setupPDP() (*application.PolicyDecisionPoint, *MemoryPermissionStore) {
	store := NewMemoryPermissionStore()

	store.AddPermission("role-viewer", entities.ActionRead, "*")

	store.AddPermission("role-editor", entities.ActionRead, "*")
	store.AddPermission("role-editor", entities.ActionModify, "*")

	store.AddPermission("role-admin", entities.ActionRead, "*")
	store.AddPermission("role-admin", entities.ActionModify, "*")
	store.AddPermission("role-admin", entities.ActionDelete, "*")

	store.AssignRole("alice", "role-admin")
	store.AssignRole("bob", "role-editor")
	store.AssignRole("carol", "role-viewer")

	store.AddPermission("dave", entities.ActionDelete, "*")
	store.AssignRole("dave", "role-editor")
	store.AddProhibition("role-editor", entities.ActionDelete, "*")

	store.AssignAccountRole("frank", "role-editor", "account-acme")

	return application.NewPolicyDecisionPoint(store), store
}

func setupCasbin(t *testing.T) *authcasbin.CasbinAuthorizationChecker {
	t.Helper()
	checker, err := authcasbin.NewCasbinAuthorizationChecker(&noopTestAdapter{})
	if err != nil {
		t.Fatalf("NewCasbinAuthorizationChecker() error: %v", err)
	}

	for _, p := range []struct{ assignee, action, target string }{
		{"role-viewer", entities.ActionRead, "*"},
		{"role-editor", entities.ActionRead, "*"},
		{"role-editor", entities.ActionModify, "*"},
		{"role-admin", entities.ActionRead, "*"},
		{"role-admin", entities.ActionModify, "*"},
		{"role-admin", entities.ActionDelete, "*"},
		{"dave", entities.ActionDelete, "*"},
	} {
		if err := checker.AddPermission(p.assignee, p.action, p.target); err != nil {
			t.Fatalf("AddPermission(%s, %s, %s) error: %v", p.assignee, p.action, p.target, err)
		}
	}

	if err := checker.AssignRole("alice", "role-admin"); err != nil {
		t.Fatalf("AssignRole error: %v", err)
	}
	if err := checker.AssignRole("bob", "role-editor"); err != nil {
		t.Fatalf("AssignRole error: %v", err)
	}
	if err := checker.AssignRole("carol", "role-viewer"); err != nil {
		t.Fatalf("AssignRole error: %v", err)
	}
	if err := checker.AssignRole("dave", "role-editor"); err != nil {
		t.Fatalf("AssignRole error: %v", err)
	}
	if err := checker.AddProhibition("role-editor", entities.ActionDelete, "*"); err != nil {
		t.Fatalf("AddProhibition error: %v", err)
	}
	if err := checker.AssignAccountRole("frank", "role-editor", "account-acme"); err != nil {
		t.Fatalf("AssignAccountRole error: %v", err)
	}

	return checker
}

func TestPDP_RoleBasedAccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pdp, _ := setupPDP()

	tests := []struct {
		agent  string
		action string
		want   bool
	}{
		// alice (admin): all allowed
		{"alice", entities.ActionRead, true},
		{"alice", entities.ActionModify, true},
		{"alice", entities.ActionDelete, true},
		// bob (editor): read+modify, not delete (prohibited)
		{"bob", entities.ActionRead, true},
		{"bob", entities.ActionModify, true},
		{"bob", entities.ActionDelete, false},
		// carol (viewer): read only
		{"carol", entities.ActionRead, true},
		{"carol", entities.ActionModify, false},
		{"carol", entities.ActionDelete, false},
	}

	for _, tt := range tests {
		t.Run(tt.agent+":"+tt.action, func(t *testing.T) {
			t.Parallel()
			ok, err := pdp.IsAuthorized(ctx, tt.agent, tt.action, "doc-1")
			if err != nil {
				t.Fatalf("IsAuthorized() error: %v", err)
			}
			if ok != tt.want {
				t.Errorf("IsAuthorized(%s, %s) = %v, want %v", tt.agent, tt.action, ok, tt.want)
			}
		})
	}
}

func TestPDP_ProhibitionOverride(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pdp, _ := setupPDP()

	// dave has direct delete permission but role-editor has delete prohibition
	ok, err := pdp.IsAuthorized(ctx, "dave", entities.ActionDelete, "doc-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected delete to be denied — prohibition should override direct permission")
	}

	// dave can still read and modify via role-editor
	ok, err = pdp.IsAuthorized(ctx, "dave", entities.ActionRead, "doc-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected read to be allowed via role-editor")
	}

	ok, err = pdp.IsAuthorized(ctx, "dave", entities.ActionModify, "doc-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected modify to be allowed via role-editor")
	}
}

func TestPDP_AccountScoped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pdp, _ := setupPDP()

	// frank has role-editor scoped to account-acme
	ok, err := pdp.IsAuthorizedInAccount(ctx, "frank", "account-acme", entities.ActionModify, "doc-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if !ok {
		t.Error("expected modify to be allowed in account-acme")
	}

	// frank has no global roles — global check should fail
	ok, err = pdp.IsAuthorized(ctx, "frank", entities.ActionModify, "doc-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected global modify to be denied — frank has no global roles")
	}

	// frank in a different account — should fail
	ok, err = pdp.IsAuthorizedInAccount(ctx, "frank", "account-other", entities.ActionModify, "doc-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if ok {
		t.Error("expected modify to be denied in account-other")
	}
}

func TestPDP_DefaultDeny(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pdp, _ := setupPDP()

	// eve has no roles or permissions — everything should be denied
	for _, action := range []string{entities.ActionRead, entities.ActionModify, entities.ActionDelete} {
		ok, err := pdp.IsAuthorized(ctx, "eve", action, "doc-1")
		if err != nil {
			t.Fatalf("IsAuthorized(eve, %s) error: %v", action, err)
		}
		if ok {
			t.Errorf("expected eve:%s to be denied (default deny)", action)
		}
	}
}

func TestCasbin_ParityWithPDP(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	pdp, _ := setupPDP()
	checker := setupCasbin(t)

	type check struct {
		agent, action, target string
	}

	checks := []check{
		{"alice", entities.ActionRead, "doc-1"},
		{"alice", entities.ActionModify, "doc-1"},
		{"alice", entities.ActionDelete, "doc-1"},
		{"bob", entities.ActionRead, "doc-1"},
		{"bob", entities.ActionModify, "doc-1"},
		{"bob", entities.ActionDelete, "doc-1"},
		{"carol", entities.ActionRead, "doc-1"},
		{"carol", entities.ActionModify, "doc-1"},
		{"carol", entities.ActionDelete, "doc-1"},
		{"dave", entities.ActionRead, "doc-1"},
		{"dave", entities.ActionModify, "doc-1"},
		{"dave", entities.ActionDelete, "doc-1"},
		{"eve", entities.ActionRead, "doc-1"},
		{"eve", entities.ActionModify, "doc-1"},
		{"eve", entities.ActionDelete, "doc-1"},
	}

	for _, c := range checks {
		t.Run(c.agent+":"+c.action, func(t *testing.T) {
			t.Parallel()
			pdpResult, err := pdp.IsAuthorized(ctx, c.agent, c.action, c.target)
			if err != nil {
				t.Fatalf("PDP IsAuthorized() error: %v", err)
			}

			casbinResult, err := checker.IsAuthorized(ctx, c.agent, c.action, c.target)
			if err != nil {
				t.Fatalf("Casbin IsAuthorized() error: %v", err)
			}

			if pdpResult != casbinResult {
				t.Errorf("%s:%s PDP=%v, Casbin=%v — parity violation",
					c.agent, c.action, pdpResult, casbinResult)
			}
		})
	}

	// Account-scoped parity for frank
	t.Run("frank:modify@acme", func(t *testing.T) {
		t.Parallel()
		pdpOk, err := pdp.IsAuthorizedInAccount(ctx, "frank", "account-acme", entities.ActionModify, "doc-1")
		if err != nil {
			t.Fatalf("PDP error: %v", err)
		}
		casbinOk, err := checker.IsAuthorizedInAccount(ctx, "frank", "account-acme", entities.ActionModify, "doc-1")
		if err != nil {
			t.Fatalf("Casbin error: %v", err)
		}
		if pdpOk != casbinOk {
			t.Errorf("frank:modify@acme PDP=%v, Casbin=%v", pdpOk, casbinOk)
		}
	})

	t.Run("frank:modify@global", func(t *testing.T) {
		t.Parallel()
		pdpOk, err := pdp.IsAuthorized(ctx, "frank", entities.ActionModify, "doc-1")
		if err != nil {
			t.Fatalf("PDP error: %v", err)
		}
		casbinOk, err := checker.IsAuthorized(ctx, "frank", entities.ActionModify, "doc-1")
		if err != nil {
			t.Fatalf("Casbin error: %v", err)
		}
		if pdpOk != casbinOk {
			t.Errorf("frank:modify@global PDP=%v, Casbin=%v", pdpOk, casbinOk)
		}
	})
}

func TestIdentityAndOwnership(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	identity := &auth.Identity{
		AgentID:         "alice",
		AccountIDs:      []string{"account-acme", "account-personal"},
		ActiveAccountID: "account-acme",
	}
	authedCtx := auth.ContextWithAgent(ctx, identity)

	// Round-trip
	recovered := auth.AgentFromCtx(authedCtx)
	if recovered == nil {
		t.Fatal("expected non-nil identity")
	}
	if recovered.AgentID != "alice" {
		t.Errorf("AgentID = %q, want %q", recovered.AgentID, "alice")
	}
	if recovered.ActiveAccountID != "account-acme" {
		t.Errorf("ActiveAccountID = %q, want %q", recovered.ActiveAccountID, "account-acme")
	}

	// ResourceOwnership
	ownership, err := auth.ResourceOwnershipFromCtx(authedCtx)
	if err != nil {
		t.Fatalf("ResourceOwnershipFromCtx() error: %v", err)
	}
	if ownership.AccountID != "account-acme" {
		t.Errorf("AccountID = %q, want %q", ownership.AccountID, "account-acme")
	}
	if ownership.CreatedByAgentID != "alice" {
		t.Errorf("CreatedByAgentID = %q, want %q", ownership.CreatedByAgentID, "alice")
	}

	// VerifyAccountAccess — same account
	if err := auth.VerifyAccountAccess(authedCtx, "account-acme"); err != nil {
		t.Fatalf("VerifyAccountAccess(same) error: %v", err)
	}

	// VerifyAccountAccess — different account
	err = auth.VerifyAccountAccess(authedCtx, "account-other")
	if !errors.Is(err, auth.ErrAccountMismatch) {
		t.Errorf("error = %v, want ErrAccountMismatch", err)
	}

	// No identity
	_, err = auth.ResourceOwnershipFromCtx(ctx)
	if !errors.Is(err, auth.ErrNoIdentity) {
		t.Errorf("error = %v, want ErrNoIdentity", err)
	}
}
