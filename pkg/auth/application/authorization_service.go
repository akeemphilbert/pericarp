package application

import (
	"context"
	"fmt"
)

// Permission represents a resolved permission or prohibition for querying.
type Permission struct {
	Assignee string // Agent or Role ID that holds this permission
	Action   string // ODRL action IRI (e.g., odrl:read)
	Target   string // Asset/resource identifier or wildcard "*"
}

// AuthorizationChecker defines the interface for authorization decisions.
// Implementations resolve agent roles, policy assignments, and evaluate
// ODRL permissions and prohibitions to reach a decision.
type AuthorizationChecker interface {
	// IsAuthorized checks whether the given agent is authorized to perform
	// the specified action on the target resource.
	// It evaluates all applicable policies, considering:
	// - Direct agent permissions
	// - Role-based permissions (via agent's assigned roles)
	// - Prohibitions (which override permissions per ODRL semantics)
	IsAuthorized(ctx context.Context, agentID, action, target string) (bool, error)

	// IsAuthorizedInAccount checks whether the given agent is authorized within
	// a specific account context. It considers:
	// - Direct agent permissions
	// - Global role-based permissions (via agent's assigned roles)
	// - Account-scoped role-based permissions (via agent's role in the account)
	// - Prohibitions (which override permissions per ODRL semantics)
	IsAuthorizedInAccount(ctx context.Context, agentID, accountID, action, target string) (bool, error)

	// GetPermissions returns all effective permissions for the given agent,
	// including permissions inherited through role assignments.
	GetPermissions(ctx context.Context, agentID string) ([]Permission, error)

	// GetProhibitions returns all effective prohibitions for the given agent,
	// including prohibitions inherited through role assignments.
	GetProhibitions(ctx context.Context, agentID string) ([]Permission, error)
}

// PermissionStore provides read access to permission data for authorization decisions.
// This interface abstracts the projection/read model that stores resolved permissions.
// Consuming applications implement this against their storage layer.
type PermissionStore interface {
	// GetPermissionsForAssignee returns all permissions for a specific assignee (agent or role).
	GetPermissionsForAssignee(ctx context.Context, assigneeID string) ([]Permission, error)

	// GetProhibitionsForAssignee returns all prohibitions for a specific assignee.
	GetProhibitionsForAssignee(ctx context.Context, assigneeID string) ([]Permission, error)

	// GetRolesForAgent returns all global role IDs currently assigned to the given agent.
	GetRolesForAgent(ctx context.Context, agentID string) ([]string, error)

	// GetRolesForAgentInAccount returns role IDs assigned to the agent within a specific account.
	GetRolesForAgentInAccount(ctx context.Context, agentID, accountID string) ([]string, error)
}

// PolicyDecisionPoint implements AuthorizationChecker using a PermissionStore
// for resolving authorization decisions following ODRL semantics.
//
// Decision logic:
//  1. Collect all assignee IDs (agent + their roles)
//  2. Check prohibitions — if any match, deny (prohibitions override permissions)
//  3. Check permissions — if any match, allow
//  4. Default deny
type PolicyDecisionPoint struct {
	store PermissionStore
}

// NewPolicyDecisionPoint creates a new PolicyDecisionPoint with the given store.
func NewPolicyDecisionPoint(store PermissionStore) *PolicyDecisionPoint {
	return &PolicyDecisionPoint{store: store}
}

// IsAuthorized checks whether the agent is authorized following ODRL semantics.
func (pdp *PolicyDecisionPoint) IsAuthorized(ctx context.Context, agentID, action, target string) (bool, error) {
	assignees, err := pdp.collectAssignees(ctx, agentID)
	if err != nil {
		return false, err
	}

	return pdp.evaluate(ctx, assignees, action, target)
}

// IsAuthorizedInAccount checks whether the agent is authorized within an account context.
// It collects both global roles and account-scoped roles before evaluating.
func (pdp *PolicyDecisionPoint) IsAuthorizedInAccount(ctx context.Context, agentID, accountID, action, target string) (bool, error) {
	assignees, err := pdp.collectAssigneesInAccount(ctx, agentID, accountID)
	if err != nil {
		return false, err
	}

	return pdp.evaluate(ctx, assignees, action, target)
}

// evaluate checks prohibitions then permissions for the given assignees.
func (pdp *PolicyDecisionPoint) evaluate(ctx context.Context, assignees []string, action, target string) (bool, error) {
	// Prohibitions override permissions (ODRL semantics)
	for _, assigneeID := range assignees {
		prohibitions, err := pdp.store.GetProhibitionsForAssignee(ctx, assigneeID)
		if err != nil {
			return false, fmt.Errorf("failed to get prohibitions for %s: %w", assigneeID, err)
		}
		for _, p := range prohibitions {
			if matchesRule(p, action, target) {
				return false, nil
			}
		}
	}

	// Check permissions
	for _, assigneeID := range assignees {
		permissions, err := pdp.store.GetPermissionsForAssignee(ctx, assigneeID)
		if err != nil {
			return false, fmt.Errorf("failed to get permissions for %s: %w", assigneeID, err)
		}
		for _, p := range permissions {
			if matchesRule(p, action, target) {
				return true, nil
			}
		}
	}

	// Default deny
	return false, nil
}

// GetPermissions returns all effective permissions for the agent.
func (pdp *PolicyDecisionPoint) GetPermissions(ctx context.Context, agentID string) ([]Permission, error) {
	assignees, err := pdp.collectAssignees(ctx, agentID)
	if err != nil {
		return nil, err
	}

	var result []Permission
	for _, assigneeID := range assignees {
		permissions, err := pdp.store.GetPermissionsForAssignee(ctx, assigneeID)
		if err != nil {
			return nil, fmt.Errorf("failed to get permissions for %s: %w", assigneeID, err)
		}
		result = append(result, permissions...)
	}
	return result, nil
}

// GetProhibitions returns all effective prohibitions for the agent.
func (pdp *PolicyDecisionPoint) GetProhibitions(ctx context.Context, agentID string) ([]Permission, error) {
	assignees, err := pdp.collectAssignees(ctx, agentID)
	if err != nil {
		return nil, err
	}

	var result []Permission
	for _, assigneeID := range assignees {
		prohibitions, err := pdp.store.GetProhibitionsForAssignee(ctx, assigneeID)
		if err != nil {
			return nil, fmt.Errorf("failed to get prohibitions for %s: %w", assigneeID, err)
		}
		result = append(result, prohibitions...)
	}
	return result, nil
}

// collectAssignees returns the agent ID plus all global role IDs assigned to the agent.
func (pdp *PolicyDecisionPoint) collectAssignees(ctx context.Context, agentID string) ([]string, error) {
	roles, err := pdp.store.GetRolesForAgent(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get roles for agent %s: %w", agentID, err)
	}

	assignees := make([]string, 0, 1+len(roles))
	assignees = append(assignees, agentID)
	assignees = append(assignees, roles...)
	return assignees, nil
}

// collectAssigneesInAccount returns the agent ID plus global roles and account-scoped roles.
func (pdp *PolicyDecisionPoint) collectAssigneesInAccount(ctx context.Context, agentID, accountID string) ([]string, error) {
	globalRoles, err := pdp.store.GetRolesForAgent(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get global roles for agent %s: %w", agentID, err)
	}

	accountRoles, err := pdp.store.GetRolesForAgentInAccount(ctx, agentID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get account roles for agent %s in account %s: %w", agentID, accountID, err)
	}

	// Deduplicate: agent + global roles + account-scoped roles
	seen := make(map[string]bool, 1+len(globalRoles)+len(accountRoles))
	assignees := make([]string, 0, 1+len(globalRoles)+len(accountRoles))

	assignees = append(assignees, agentID)
	seen[agentID] = true

	for _, r := range globalRoles {
		if !seen[r] {
			assignees = append(assignees, r)
			seen[r] = true
		}
	}
	for _, r := range accountRoles {
		if !seen[r] {
			assignees = append(assignees, r)
			seen[r] = true
		}
	}

	return assignees, nil
}

// matchesRule checks if a permission/prohibition matches the requested action and target.
// Supports wildcard "*" for target matching.
func matchesRule(p Permission, action, target string) bool {
	if p.Action != action {
		return false
	}
	return p.Target == target || p.Target == "*"
}
