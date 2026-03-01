package casbin_test

import (
	"context"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	authcasbin "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/casbin"
	"github.com/casbin/casbin/v3/model"
)

// noopAdapter implements persist.Adapter for testing without persistence.
type noopAdapter struct{}

func (a *noopAdapter) LoadPolicy(model.Model) error                              { return nil }
func (a *noopAdapter) SavePolicy(model.Model) error                              { return nil }
func (a *noopAdapter) AddPolicy(string, string, []string) error                  { return nil }
func (a *noopAdapter) RemovePolicy(string, string, []string) error               { return nil }
func (a *noopAdapter) RemoveFilteredPolicy(string, string, int, ...string) error { return nil }

func newTestChecker(t *testing.T) *authcasbin.CasbinAuthorizationChecker {
	t.Helper()
	checker, err := authcasbin.NewCasbinAuthorizationChecker(&noopAdapter{})
	if err != nil {
		t.Fatalf("NewCasbinAuthorizationChecker() error: %v", err)
	}
	return checker
}

func TestCasbinAuthorizationChecker_DirectPermission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	checker := newTestChecker(t)

	if err := checker.AddPermission("agent-1", entities.ActionRead, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}

	// Permitted action
	ok, err := checker.IsAuthorized(ctx, "agent-1", entities.ActionRead, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected authorization to succeed")
	}

	// Non-permitted action
	ok, err = checker.IsAuthorized(ctx, "agent-1", entities.ActionDelete, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected authorization to fail for non-permitted action")
	}

	// Non-permitted target
	ok, err = checker.IsAuthorized(ctx, "agent-1", entities.ActionRead, "resource-2")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected authorization to fail for non-permitted target")
	}
}

func TestCasbinAuthorizationChecker_RoleBasedPermission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	checker := newTestChecker(t)

	if err := checker.AssignRole("agent-1", "role-admin"); err != nil {
		t.Fatalf("AssignRole() error: %v", err)
	}
	if err := checker.AddPermission("role-admin", entities.ActionRead, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}
	if err := checker.AddPermission("role-admin", entities.ActionModify, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}

	// Agent inherits permissions from role
	ok, err := checker.IsAuthorized(ctx, "agent-1", entities.ActionRead, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected authorization via role to succeed")
	}

	ok, err = checker.IsAuthorized(ctx, "agent-1", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected authorization via role to succeed for modify")
	}
}

func TestCasbinAuthorizationChecker_ProhibitionOverridesPermission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	checker := newTestChecker(t)

	if err := checker.AddPermission("agent-1", entities.ActionRead, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}
	if err := checker.AddPermission("agent-1", entities.ActionDelete, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}
	if err := checker.AddProhibition("agent-1", entities.ActionDelete, "resource-1"); err != nil {
		t.Fatalf("AddProhibition() error: %v", err)
	}

	// Read is permitted (no prohibition)
	ok, err := checker.IsAuthorized(ctx, "agent-1", entities.ActionRead, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected read to be authorized")
	}

	// Delete is prohibited (prohibition overrides permission)
	ok, err = checker.IsAuthorized(ctx, "agent-1", entities.ActionDelete, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected delete to be denied due to prohibition")
	}
}

func TestCasbinAuthorizationChecker_WildcardTarget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	checker := newTestChecker(t)

	if err := checker.AddPermission("agent-1", entities.ActionRead, "*"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}

	// Wildcard target matches any resource
	ok, err := checker.IsAuthorized(ctx, "agent-1", entities.ActionRead, "any-resource")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected wildcard permission to authorize any target")
	}
}

func TestCasbinAuthorizationChecker_DefaultDeny(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	checker := newTestChecker(t)

	// No permissions at all — default deny
	ok, err := checker.IsAuthorized(ctx, "agent-1", entities.ActionRead, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected default deny when no permissions exist")
	}
}

func TestCasbinAuthorizationChecker_RoleProhibitionOverridesDirectPermission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	checker := newTestChecker(t)

	if err := checker.AssignRole("agent-1", "role-restricted"); err != nil {
		t.Fatalf("AssignRole() error: %v", err)
	}
	// Agent has direct permission
	if err := checker.AddPermission("agent-1", entities.ActionDelete, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}
	// But role has prohibition
	if err := checker.AddProhibition("role-restricted", entities.ActionDelete, "resource-1"); err != nil {
		t.Fatalf("AddProhibition() error: %v", err)
	}

	// Prohibition from role overrides direct permission
	ok, err := checker.IsAuthorized(ctx, "agent-1", entities.ActionDelete, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected role prohibition to override direct permission")
	}
}

func TestCasbinAuthorizationChecker_GetPermissions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	checker := newTestChecker(t)

	if err := checker.AssignRole("agent-1", "role-admin"); err != nil {
		t.Fatalf("AssignRole() error: %v", err)
	}
	if err := checker.AddPermission("agent-1", entities.ActionRead, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}
	if err := checker.AddPermission("role-admin", entities.ActionModify, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}

	perms, err := checker.GetPermissions(ctx, "agent-1")
	if err != nil {
		t.Fatalf("GetPermissions() error: %v", err)
	}
	if len(perms) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(perms))
	}
}

func TestCasbinAuthorizationChecker_GetProhibitions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	checker := newTestChecker(t)

	if err := checker.AssignRole("agent-1", "role-restricted"); err != nil {
		t.Fatalf("AssignRole() error: %v", err)
	}
	if err := checker.AddProhibition("agent-1", entities.ActionDelete, "resource-1"); err != nil {
		t.Fatalf("AddProhibition() error: %v", err)
	}
	if err := checker.AddProhibition("role-restricted", entities.ActionExecute, "*"); err != nil {
		t.Fatalf("AddProhibition() error: %v", err)
	}

	prohibitions, err := checker.GetProhibitions(ctx, "agent-1")
	if err != nil {
		t.Fatalf("GetProhibitions() error: %v", err)
	}
	if len(prohibitions) != 2 {
		t.Fatalf("expected 2 prohibitions, got %d", len(prohibitions))
	}
}

func TestCasbinAuthorizationChecker_AccountScopedRole(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	checker := newTestChecker(t)

	// Agent has no global roles, but has "role-editor" in "account-1"
	if err := checker.AssignAccountRole("agent-1", "role-editor", "account-1"); err != nil {
		t.Fatalf("AssignAccountRole() error: %v", err)
	}
	if err := checker.AddPermission("role-editor", entities.ActionModify, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}

	// Global check: no permission (agent has no global roles)
	ok, err := checker.IsAuthorized(ctx, "agent-1", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected global authorization to fail (no global roles)")
	}

	// Account-scoped check: permitted via account role
	ok, err = checker.IsAuthorizedInAccount(ctx, "agent-1", "account-1", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if !ok {
		t.Error("expected account-scoped authorization to succeed via account role")
	}

	// Different account: no permission
	ok, err = checker.IsAuthorizedInAccount(ctx, "agent-1", "account-2", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if ok {
		t.Error("expected authorization to fail in different account")
	}
}

func TestCasbinAuthorizationChecker_AccountAndGlobalRolesCombined(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	checker := newTestChecker(t)

	// Global role gives read access
	if err := checker.AssignRole("agent-1", "role-viewer"); err != nil {
		t.Fatalf("AssignRole() error: %v", err)
	}
	if err := checker.AddPermission("role-viewer", entities.ActionRead, "*"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}
	// Account role gives modify access
	if err := checker.AssignAccountRole("agent-1", "role-editor", "account-1"); err != nil {
		t.Fatalf("AssignAccountRole() error: %v", err)
	}
	if err := checker.AddPermission("role-editor", entities.ActionModify, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}

	// In account context, agent should have both read (global) and modify (account)
	ok, err := checker.IsAuthorizedInAccount(ctx, "agent-1", "account-1", entities.ActionRead, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if !ok {
		t.Error("expected read to succeed via global role in account context")
	}

	ok, err = checker.IsAuthorizedInAccount(ctx, "agent-1", "account-1", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if !ok {
		t.Error("expected modify to succeed via account role")
	}
}

func TestCasbinAuthorizationChecker_AccountProhibitionOverrides(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	checker := newTestChecker(t)

	// Agent has direct permission to delete
	if err := checker.AddPermission("agent-1", entities.ActionDelete, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}
	// Account role prohibits delete
	if err := checker.AssignAccountRole("agent-1", "role-restricted", "account-1"); err != nil {
		t.Fatalf("AssignAccountRole() error: %v", err)
	}
	if err := checker.AddProhibition("role-restricted", entities.ActionDelete, "resource-1"); err != nil {
		t.Fatalf("AddProhibition() error: %v", err)
	}

	// In account context, prohibition from account role overrides direct permission
	ok, err := checker.IsAuthorizedInAccount(ctx, "agent-1", "account-1", entities.ActionDelete, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if ok {
		t.Error("expected account role prohibition to override direct permission")
	}
}

func TestCasbinAuthorizationChecker_AccountRoleDeduplication(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	checker := newTestChecker(t)

	// Same role assigned both globally and in account
	if err := checker.AssignRole("agent-1", "role-admin"); err != nil {
		t.Fatalf("AssignRole() error: %v", err)
	}
	if err := checker.AssignAccountRole("agent-1", "role-admin", "account-1"); err != nil {
		t.Fatalf("AssignAccountRole() error: %v", err)
	}
	if err := checker.AddPermission("role-admin", entities.ActionRead, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}

	// Should still work (deduplicated, not double-counted)
	perms, err := checker.GetPermissions(ctx, "agent-1")
	if err != nil {
		t.Fatalf("GetPermissions() error: %v", err)
	}
	// Only 1 permission from role-admin (not duplicated)
	if len(perms) != 1 {
		t.Fatalf("expected 1 permission (deduplicated), got %d", len(perms))
	}
}

func TestCasbinAuthorizationChecker_ConvenienceMethods(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	checker := newTestChecker(t)

	// Add permission and verify
	if err := checker.AddPermission("agent-1", entities.ActionRead, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}
	ok, err := checker.IsAuthorized(ctx, "agent-1", entities.ActionRead, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected permission to be granted after AddPermission")
	}

	// Remove permission and verify
	if err := checker.RemovePermission("agent-1", entities.ActionRead, "resource-1"); err != nil {
		t.Fatalf("RemovePermission() error: %v", err)
	}
	ok, err = checker.IsAuthorized(ctx, "agent-1", entities.ActionRead, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected permission to be revoked after RemovePermission")
	}

	// Add prohibition and verify
	if err := checker.AddPermission("agent-1", entities.ActionDelete, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}
	if err := checker.AddProhibition("agent-1", entities.ActionDelete, "resource-1"); err != nil {
		t.Fatalf("AddProhibition() error: %v", err)
	}
	ok, err = checker.IsAuthorized(ctx, "agent-1", entities.ActionDelete, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected prohibition to deny despite permission")
	}

	// Remove prohibition and verify
	if err := checker.RemoveProhibition("agent-1", entities.ActionDelete, "resource-1"); err != nil {
		t.Fatalf("RemoveProhibition() error: %v", err)
	}
	ok, err = checker.IsAuthorized(ctx, "agent-1", entities.ActionDelete, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected permission to work after prohibition removal")
	}

	// Assign and revoke global role
	if err := checker.AddPermission("role-editor", entities.ActionModify, "resource-1"); err != nil {
		t.Fatalf("AddPermission() error: %v", err)
	}
	if err := checker.AssignRole("agent-2", "role-editor"); err != nil {
		t.Fatalf("AssignRole() error: %v", err)
	}
	ok, err = checker.IsAuthorized(ctx, "agent-2", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected agent-2 to inherit role-editor permission")
	}

	if err := checker.RevokeRole("agent-2", "role-editor"); err != nil {
		t.Fatalf("RevokeRole() error: %v", err)
	}
	ok, err = checker.IsAuthorized(ctx, "agent-2", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected agent-2 to lose role-editor permission after revocation")
	}

	// Assign and revoke account role
	if err := checker.AssignAccountRole("agent-3", "role-editor", "account-1"); err != nil {
		t.Fatalf("AssignAccountRole() error: %v", err)
	}
	ok, err = checker.IsAuthorizedInAccount(ctx, "agent-3", "account-1", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if !ok {
		t.Error("expected agent-3 to have account role permission")
	}

	if err := checker.RevokeAccountRole("agent-3", "role-editor", "account-1"); err != nil {
		t.Fatalf("RevokeAccountRole() error: %v", err)
	}
	ok, err = checker.IsAuthorizedInAccount(ctx, "agent-3", "account-1", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if ok {
		t.Error("expected agent-3 to lose account role permission after revocation")
	}
}
