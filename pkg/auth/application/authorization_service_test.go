package application_test

import (
	"context"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

// mockPermissionStore implements PermissionStore for testing.
type mockPermissionStore struct {
	permissions  map[string][]application.Permission
	prohibitions map[string][]application.Permission
	roles        map[string][]string
	accountRoles map[string]map[string][]string // agentID -> accountID -> roleIDs
}

func newMockStore() *mockPermissionStore {
	return &mockPermissionStore{
		permissions:  make(map[string][]application.Permission),
		prohibitions: make(map[string][]application.Permission),
		roles:        make(map[string][]string),
		accountRoles: make(map[string]map[string][]string),
	}
}

func (m *mockPermissionStore) GetPermissionsForAssignee(_ context.Context, assigneeID string) ([]application.Permission, error) {
	return m.permissions[assigneeID], nil
}

func (m *mockPermissionStore) GetProhibitionsForAssignee(_ context.Context, assigneeID string) ([]application.Permission, error) {
	return m.prohibitions[assigneeID], nil
}

func (m *mockPermissionStore) GetRolesForAgent(_ context.Context, agentID string) ([]string, error) {
	return m.roles[agentID], nil
}

func (m *mockPermissionStore) GetRolesForAgentInAccount(_ context.Context, agentID, accountID string) ([]string, error) {
	if accounts, ok := m.accountRoles[agentID]; ok {
		return accounts[accountID], nil
	}
	return nil, nil
}

func (m *mockPermissionStore) setAccountRoles(agentID, accountID string, roles []string) {
	if m.accountRoles[agentID] == nil {
		m.accountRoles[agentID] = make(map[string][]string)
	}
	m.accountRoles[agentID][accountID] = roles
}

func TestPolicyDecisionPoint_DirectPermission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := newMockStore()
	store.permissions["agent-1"] = []application.Permission{
		{Assignee: "agent-1", Action: entities.ActionRead, Target: "resource-1"},
	}

	pdp := application.NewPolicyDecisionPoint(store)

	// Permitted action
	ok, err := pdp.IsAuthorized(ctx, "agent-1", entities.ActionRead, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected authorization to succeed")
	}

	// Non-permitted action
	ok, err = pdp.IsAuthorized(ctx, "agent-1", entities.ActionDelete, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected authorization to fail for non-permitted action")
	}

	// Non-permitted target
	ok, err = pdp.IsAuthorized(ctx, "agent-1", entities.ActionRead, "resource-2")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected authorization to fail for non-permitted target")
	}
}

func TestPolicyDecisionPoint_RoleBasedPermission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := newMockStore()
	store.roles["agent-1"] = []string{"role-admin"}
	store.permissions["role-admin"] = []application.Permission{
		{Assignee: "role-admin", Action: entities.ActionRead, Target: "resource-1"},
		{Assignee: "role-admin", Action: entities.ActionModify, Target: "resource-1"},
	}

	pdp := application.NewPolicyDecisionPoint(store)

	// Agent inherits permissions from role
	ok, err := pdp.IsAuthorized(ctx, "agent-1", entities.ActionRead, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected authorization via role to succeed")
	}

	ok, err = pdp.IsAuthorized(ctx, "agent-1", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected authorization via role to succeed for modify")
	}
}

func TestPolicyDecisionPoint_ProhibitionOverridesPermission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := newMockStore()
	store.permissions["agent-1"] = []application.Permission{
		{Assignee: "agent-1", Action: entities.ActionRead, Target: "resource-1"},
		{Assignee: "agent-1", Action: entities.ActionDelete, Target: "resource-1"},
	}
	store.prohibitions["agent-1"] = []application.Permission{
		{Assignee: "agent-1", Action: entities.ActionDelete, Target: "resource-1"},
	}

	pdp := application.NewPolicyDecisionPoint(store)

	// Read is permitted (no prohibition)
	ok, err := pdp.IsAuthorized(ctx, "agent-1", entities.ActionRead, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected read to be authorized")
	}

	// Delete is prohibited (prohibition overrides permission)
	ok, err = pdp.IsAuthorized(ctx, "agent-1", entities.ActionDelete, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected delete to be denied due to prohibition")
	}
}

func TestPolicyDecisionPoint_WildcardTarget(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := newMockStore()
	store.permissions["agent-1"] = []application.Permission{
		{Assignee: "agent-1", Action: entities.ActionRead, Target: "*"},
	}

	pdp := application.NewPolicyDecisionPoint(store)

	// Wildcard target matches any resource
	ok, err := pdp.IsAuthorized(ctx, "agent-1", entities.ActionRead, "any-resource")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if !ok {
		t.Error("expected wildcard permission to authorize any target")
	}
}

func TestPolicyDecisionPoint_DefaultDeny(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := newMockStore()
	pdp := application.NewPolicyDecisionPoint(store)

	// No permissions at all — default deny
	ok, err := pdp.IsAuthorized(ctx, "agent-1", entities.ActionRead, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected default deny when no permissions exist")
	}
}

func TestPolicyDecisionPoint_RoleProhibitionOverridesDirectPermission(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := newMockStore()
	store.roles["agent-1"] = []string{"role-restricted"}
	// Agent has direct permission
	store.permissions["agent-1"] = []application.Permission{
		{Assignee: "agent-1", Action: entities.ActionDelete, Target: "resource-1"},
	}
	// But role has prohibition
	store.prohibitions["role-restricted"] = []application.Permission{
		{Assignee: "role-restricted", Action: entities.ActionDelete, Target: "resource-1"},
	}

	pdp := application.NewPolicyDecisionPoint(store)

	// Prohibition from role overrides direct permission
	ok, err := pdp.IsAuthorized(ctx, "agent-1", entities.ActionDelete, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected role prohibition to override direct permission")
	}
}

func TestPolicyDecisionPoint_GetPermissions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := newMockStore()
	store.roles["agent-1"] = []string{"role-admin"}
	store.permissions["agent-1"] = []application.Permission{
		{Assignee: "agent-1", Action: entities.ActionRead, Target: "resource-1"},
	}
	store.permissions["role-admin"] = []application.Permission{
		{Assignee: "role-admin", Action: entities.ActionModify, Target: "resource-1"},
	}

	pdp := application.NewPolicyDecisionPoint(store)

	perms, err := pdp.GetPermissions(ctx, "agent-1")
	if err != nil {
		t.Fatalf("GetPermissions() error: %v", err)
	}
	if len(perms) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(perms))
	}
}

func TestPolicyDecisionPoint_GetProhibitions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := newMockStore()
	store.roles["agent-1"] = []string{"role-restricted"}
	store.prohibitions["agent-1"] = []application.Permission{
		{Assignee: "agent-1", Action: entities.ActionDelete, Target: "resource-1"},
	}
	store.prohibitions["role-restricted"] = []application.Permission{
		{Assignee: "role-restricted", Action: entities.ActionExecute, Target: "*"},
	}

	pdp := application.NewPolicyDecisionPoint(store)

	prohibitions, err := pdp.GetProhibitions(ctx, "agent-1")
	if err != nil {
		t.Fatalf("GetProhibitions() error: %v", err)
	}
	if len(prohibitions) != 2 {
		t.Fatalf("expected 2 prohibitions, got %d", len(prohibitions))
	}
}

func TestPolicyDecisionPoint_AccountScopedRole(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := newMockStore()
	// Agent has no global roles
	// But has "role-editor" in "account-1"
	store.setAccountRoles("agent-1", "account-1", []string{"role-editor"})
	store.permissions["role-editor"] = []application.Permission{
		{Assignee: "role-editor", Action: entities.ActionModify, Target: "resource-1"},
	}

	pdp := application.NewPolicyDecisionPoint(store)

	// Global check: no permission (agent has no global roles)
	ok, err := pdp.IsAuthorized(ctx, "agent-1", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorized() error: %v", err)
	}
	if ok {
		t.Error("expected global authorization to fail (no global roles)")
	}

	// Account-scoped check: permitted via account role
	ok, err = pdp.IsAuthorizedInAccount(ctx, "agent-1", "account-1", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if !ok {
		t.Error("expected account-scoped authorization to succeed via account role")
	}

	// Different account: no permission
	ok, err = pdp.IsAuthorizedInAccount(ctx, "agent-1", "account-2", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if ok {
		t.Error("expected authorization to fail in different account")
	}
}

func TestPolicyDecisionPoint_AccountAndGlobalRolesCombined(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := newMockStore()
	// Global role gives read access
	store.roles["agent-1"] = []string{"role-viewer"}
	store.permissions["role-viewer"] = []application.Permission{
		{Assignee: "role-viewer", Action: entities.ActionRead, Target: "*"},
	}
	// Account role gives modify access
	store.setAccountRoles("agent-1", "account-1", []string{"role-editor"})
	store.permissions["role-editor"] = []application.Permission{
		{Assignee: "role-editor", Action: entities.ActionModify, Target: "resource-1"},
	}

	pdp := application.NewPolicyDecisionPoint(store)

	// In account context, agent should have both read (global) and modify (account)
	ok, err := pdp.IsAuthorizedInAccount(ctx, "agent-1", "account-1", entities.ActionRead, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if !ok {
		t.Error("expected read to succeed via global role in account context")
	}

	ok, err = pdp.IsAuthorizedInAccount(ctx, "agent-1", "account-1", entities.ActionModify, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if !ok {
		t.Error("expected modify to succeed via account role")
	}
}

func TestPolicyDecisionPoint_AccountProhibitionOverrides(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := newMockStore()
	// Agent has direct permission to delete
	store.permissions["agent-1"] = []application.Permission{
		{Assignee: "agent-1", Action: entities.ActionDelete, Target: "resource-1"},
	}
	// Account role prohibits delete
	store.setAccountRoles("agent-1", "account-1", []string{"role-restricted"})
	store.prohibitions["role-restricted"] = []application.Permission{
		{Assignee: "role-restricted", Action: entities.ActionDelete, Target: "resource-1"},
	}

	pdp := application.NewPolicyDecisionPoint(store)

	// In account context, prohibition from account role overrides direct permission
	ok, err := pdp.IsAuthorizedInAccount(ctx, "agent-1", "account-1", entities.ActionDelete, "resource-1")
	if err != nil {
		t.Fatalf("IsAuthorizedInAccount() error: %v", err)
	}
	if ok {
		t.Error("expected account role prohibition to override direct permission")
	}
}

func TestPolicyDecisionPoint_AccountRoleDeduplication(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := newMockStore()
	// Same role assigned both globally and in account
	store.roles["agent-1"] = []string{"role-admin"}
	store.setAccountRoles("agent-1", "account-1", []string{"role-admin"})
	store.permissions["role-admin"] = []application.Permission{
		{Assignee: "role-admin", Action: entities.ActionRead, Target: "resource-1"},
	}

	pdp := application.NewPolicyDecisionPoint(store)

	// Should still work (deduplicated, not double-counted)
	perms, err := pdp.GetPermissions(ctx, "agent-1")
	if err != nil {
		t.Fatalf("GetPermissions() error: %v", err)
	}
	// Only 1 permission from role-admin (not duplicated)
	if len(perms) != 1 {
		t.Fatalf("expected 1 permission (deduplicated), got %d", len(perms))
	}
}
