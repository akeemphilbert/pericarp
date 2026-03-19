// Package main demonstrates the Pericarp authorization system.
//
// Run with:
//
//	go run ./examples/authz/
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/akeemphilbert/pericarp/pkg/auth"
	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	authcasbin "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/casbin"
	"github.com/casbin/casbin/v3/model"
)

// noopAdapter implements persist.Adapter for in-memory Casbin usage.
type noopAdapter struct{}

func (a *noopAdapter) LoadPolicy(model.Model) error                              { return nil }
func (a *noopAdapter) SavePolicy(model.Model) error                              { return nil }
func (a *noopAdapter) AddPolicy(string, string, []string) error                  { return nil }
func (a *noopAdapter) RemovePolicy(string, string, []string) error               { return nil }
func (a *noopAdapter) RemoveFilteredPolicy(string, string, int, ...string) error { return nil }

// DemoResult captures all authorization decisions for testing.
type DemoResult struct {
	PDP    map[string]bool
	Casbin map[string]bool
}

// RunAuthorizationDemo runs the authorization demo and returns all decisions.
func RunAuthorizationDemo(ctx context.Context) (*DemoResult, error) {
	result := &DemoResult{
		PDP:    make(map[string]bool),
		Casbin: make(map[string]bool),
	}

	// ===== Part A: PolicyDecisionPoint =====
	fmt.Println("=== Part A: PolicyDecisionPoint ===")
	fmt.Println()

	store := NewMemoryPermissionStore()

	// Define role permissions for a document management scenario
	// role-viewer: read
	store.AddPermission("role-viewer", entities.ActionRead, "*")

	// role-editor: read + modify
	store.AddPermission("role-editor", entities.ActionRead, "*")
	store.AddPermission("role-editor", entities.ActionModify, "*")

	// role-admin: read + modify + delete
	store.AddPermission("role-admin", entities.ActionRead, "*")
	store.AddPermission("role-admin", entities.ActionModify, "*")
	store.AddPermission("role-admin", entities.ActionDelete, "*")

	// alice → role-admin → can read/modify/delete
	store.AssignRole("alice", "role-admin")

	// bob → role-editor → can read/modify, not delete
	store.AssignRole("bob", "role-editor")

	// carol → role-viewer → can read only
	store.AssignRole("carol", "role-viewer")

	// dave → direct delete permission + role-editor with delete prohibition → prohibition wins
	store.AddPermission("dave", entities.ActionDelete, "*")
	store.AssignRole("dave", "role-editor")
	store.AddProhibition("role-editor", entities.ActionDelete, "*")

	// eve → no roles → default deny

	// frank → role-editor scoped to account-acme
	store.AssignAccountRole("frank", "role-editor", "account-acme")

	pdp := application.NewPolicyDecisionPoint(store)

	// Test all agent/action combinations
	type check struct {
		agent, action, target string
		label                 string
	}

	checks := []check{
		{"alice", entities.ActionRead, "doc-1", "alice:read"},
		{"alice", entities.ActionModify, "doc-1", "alice:modify"},
		{"alice", entities.ActionDelete, "doc-1", "alice:delete"},
		{"bob", entities.ActionRead, "doc-1", "bob:read"},
		{"bob", entities.ActionModify, "doc-1", "bob:modify"},
		{"bob", entities.ActionDelete, "doc-1", "bob:delete"},
		{"carol", entities.ActionRead, "doc-1", "carol:read"},
		{"carol", entities.ActionModify, "doc-1", "carol:modify"},
		{"carol", entities.ActionDelete, "doc-1", "carol:delete"},
		{"dave", entities.ActionRead, "doc-1", "dave:read"},
		{"dave", entities.ActionModify, "doc-1", "dave:modify"},
		{"dave", entities.ActionDelete, "doc-1", "dave:delete"},
		{"eve", entities.ActionRead, "doc-1", "eve:read"},
		{"eve", entities.ActionModify, "doc-1", "eve:modify"},
		{"eve", entities.ActionDelete, "doc-1", "eve:delete"},
	}

	for _, c := range checks {
		ok, err := pdp.IsAuthorized(ctx, c.agent, c.action, c.target)
		if err != nil {
			return nil, fmt.Errorf("PDP IsAuthorized(%s): %w", c.label, err)
		}
		result.PDP[c.label] = ok
		fmt.Printf("  PDP %-20s → %v\n", c.label, ok)
	}

	// Account-scoped checks for frank
	frankAccountChecks := []struct {
		accountID string
		action    string
		label     string
	}{
		{"account-acme", entities.ActionModify, "frank:modify@acme"},
		{"account-acme", entities.ActionRead, "frank:read@acme"},
		{"*", entities.ActionModify, "frank:modify@global"},
	}

	for _, c := range frankAccountChecks {
		var ok bool
		var err error
		if c.accountID == "*" {
			ok, err = pdp.IsAuthorized(ctx, "frank", c.action, "doc-1")
		} else {
			ok, err = pdp.IsAuthorizedInAccount(ctx, "frank", c.accountID, c.action, "doc-1")
		}
		if err != nil {
			return nil, fmt.Errorf("PDP %s: %w", c.label, err)
		}
		result.PDP[c.label] = ok
		fmt.Printf("  PDP %-20s → %v\n", c.label, ok)
	}

	// ===== Part B: CasbinAuthorizationChecker =====
	fmt.Println()
	fmt.Println("=== Part B: CasbinAuthorizationChecker ===")
	fmt.Println()

	checker, err := authcasbin.NewCasbinAuthorizationChecker(&noopAdapter{})
	if err != nil {
		return nil, fmt.Errorf("NewCasbinAuthorizationChecker: %w", err)
	}

	// Set up the same scenario using Casbin convenience methods
	// role-viewer permissions
	if err := checker.AddPermission("role-viewer", entities.ActionRead, "*"); err != nil {
		return nil, fmt.Errorf("AddPermission: %w", err)
	}
	// role-editor permissions
	if err := checker.AddPermission("role-editor", entities.ActionRead, "*"); err != nil {
		return nil, fmt.Errorf("AddPermission: %w", err)
	}
	if err := checker.AddPermission("role-editor", entities.ActionModify, "*"); err != nil {
		return nil, fmt.Errorf("AddPermission: %w", err)
	}
	// role-admin permissions
	if err := checker.AddPermission("role-admin", entities.ActionRead, "*"); err != nil {
		return nil, fmt.Errorf("AddPermission: %w", err)
	}
	if err := checker.AddPermission("role-admin", entities.ActionModify, "*"); err != nil {
		return nil, fmt.Errorf("AddPermission: %w", err)
	}
	if err := checker.AddPermission("role-admin", entities.ActionDelete, "*"); err != nil {
		return nil, fmt.Errorf("AddPermission: %w", err)
	}

	// Role assignments
	if err := checker.AssignRole("alice", "role-admin"); err != nil {
		return nil, fmt.Errorf("AssignRole: %w", err)
	}
	if err := checker.AssignRole("bob", "role-editor"); err != nil {
		return nil, fmt.Errorf("AssignRole: %w", err)
	}
	if err := checker.AssignRole("carol", "role-viewer"); err != nil {
		return nil, fmt.Errorf("AssignRole: %w", err)
	}

	// dave: direct delete + role-editor + prohibition on role-editor delete
	if err := checker.AddPermission("dave", entities.ActionDelete, "*"); err != nil {
		return nil, fmt.Errorf("AddPermission: %w", err)
	}
	if err := checker.AssignRole("dave", "role-editor"); err != nil {
		return nil, fmt.Errorf("AssignRole: %w", err)
	}
	if err := checker.AddProhibition("role-editor", entities.ActionDelete, "*"); err != nil {
		return nil, fmt.Errorf("AddProhibition: %w", err)
	}

	// frank: account-scoped role-editor
	if err := checker.AssignAccountRole("frank", "role-editor", "account-acme"); err != nil {
		return nil, fmt.Errorf("AssignAccountRole: %w", err)
	}

	// Run the same checks against Casbin
	for _, c := range checks {
		ok, err := checker.IsAuthorized(ctx, c.agent, c.action, c.target)
		if err != nil {
			return nil, fmt.Errorf("Casbin IsAuthorized(%s): %w", c.label, err)
		}
		result.Casbin[c.label] = ok
		fmt.Printf("  Casbin %-20s → %v\n", c.label, ok)
	}

	// Frank's account-scoped checks via Casbin
	frankModifyAcme, err := checker.IsAuthorizedInAccount(ctx, "frank", "account-acme", entities.ActionModify, "doc-1")
	if err != nil {
		return nil, fmt.Errorf("Casbin frank:modify@acme: %w", err)
	}
	result.Casbin["frank:modify@acme"] = frankModifyAcme
	fmt.Printf("  Casbin %-20s → %v\n", "frank:modify@acme", frankModifyAcme)

	frankReadAcme, err := checker.IsAuthorizedInAccount(ctx, "frank", "account-acme", entities.ActionRead, "doc-1")
	if err != nil {
		return nil, fmt.Errorf("Casbin frank:read@acme: %w", err)
	}
	result.Casbin["frank:read@acme"] = frankReadAcme
	fmt.Printf("  Casbin %-20s → %v\n", "frank:read@acme", frankReadAcme)

	frankModifyGlobal, err := checker.IsAuthorized(ctx, "frank", entities.ActionModify, "doc-1")
	if err != nil {
		return nil, fmt.Errorf("Casbin frank:modify@global: %w", err)
	}
	result.Casbin["frank:modify@global"] = frankModifyGlobal
	fmt.Printf("  Casbin %-20s → %v\n", "frank:modify@global", frankModifyGlobal)

	// ===== Identity + Ownership Integration =====
	fmt.Println()
	fmt.Println("=== Identity + Ownership ===")
	fmt.Println()

	identity := &auth.Identity{
		AgentID:         "alice",
		AccountIDs:      []string{"account-acme"},
		ActiveAccountID: "account-acme",
	}
	authedCtx := auth.ContextWithAgent(ctx, identity)

	recovered := auth.AgentFromCtx(authedCtx)
	fmt.Printf("  Identity: AgentID=%s, ActiveAccount=%s\n", recovered.AgentID, recovered.ActiveAccountID)

	ownership, err := auth.ResourceOwnershipFromCtx(authedCtx)
	if err != nil {
		return nil, fmt.Errorf("ResourceOwnershipFromCtx: %w", err)
	}
	fmt.Printf("  Ownership: AccountID=%s, CreatedBy=%s\n", ownership.AccountID, ownership.CreatedByAgentID)

	return result, nil
}

func main() {
	ctx := context.Background()
	fmt.Println("=== Pericarp Authorization Demo ===")
	fmt.Println()

	_, err := RunAuthorizationDemo(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("=== All authorization checks completed ===")
}
