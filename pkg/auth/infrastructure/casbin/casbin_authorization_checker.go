package casbin

import (
	"context"
	"fmt"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	casbinlib "github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
)

// casbinModel defines an RBAC model with domains and deny-override policy effect.
// This maps ODRL semantics to Casbin:
//   - Permissions → policies with eft=allow
//   - Prohibitions → policies with eft=deny
//   - Prohibition overrides permission → deny-override policy effect
//   - Global roles → grouping policies with domain "*"
//   - Account-scoped roles → grouping policies with domain=accountID
const casbinModel = `[request_definition]
r = sub, dom, act, obj

[policy_definition]
p = sub, dom, act, obj, eft

[role_definition]
g = _, _, _

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = g(r.sub, p.sub, r.dom) && r.act == p.act && (r.obj == p.obj || p.obj == "*") && (r.dom == p.dom || p.dom == "*")
`

// CasbinAuthorizationChecker implements application.AuthorizationChecker
// using Casbin's enforcement engine with RBAC and domain support.
type CasbinAuthorizationChecker struct {
	enforcer *casbinlib.Enforcer
}

// NewCasbinAuthorizationChecker creates a CasbinAuthorizationChecker with the
// embedded ODRL-compatible model and the given adapter for policy persistence.
func NewCasbinAuthorizationChecker(adapter persist.Adapter) (*CasbinAuthorizationChecker, error) {
	m, err := model.NewModelFromString(casbinModel)
	if err != nil {
		return nil, fmt.Errorf("failed to create casbin model: %w", err)
	}

	e, err := casbinlib.NewEnforcer(m, adapter)
	if err != nil {
		return nil, fmt.Errorf("failed to create casbin enforcer: %w", err)
	}

	e.AddNamedDomainMatchingFunc("g", "domain", func(request, stored string) bool {
		if stored == "*" {
			return true // global role/policy applies to any request domain
		}
		return stored == request
	})

	return &CasbinAuthorizationChecker{enforcer: e}, nil
}

// NewCasbinAuthorizationCheckerFromEnforcer wraps a pre-configured Casbin enforcer.
// The caller is responsible for configuring the model, adapter, and domain matching.
func NewCasbinAuthorizationCheckerFromEnforcer(enforcer *casbinlib.Enforcer) *CasbinAuthorizationChecker {
	return &CasbinAuthorizationChecker{enforcer: enforcer}
}

// IsAuthorized checks whether the agent is authorized to perform the action on
// the target using only global roles and policies.
func (c *CasbinAuthorizationChecker) IsAuthorized(_ context.Context, agentID, action, target string) (bool, error) {
	return c.enforcer.Enforce(agentID, "*", action, target)
}

// IsAuthorizedInAccount checks whether the agent is authorized within an account
// context. Both global and account-scoped roles are considered.
func (c *CasbinAuthorizationChecker) IsAuthorizedInAccount(_ context.Context, agentID, accountID, action, target string) (bool, error) {
	return c.enforcer.Enforce(agentID, accountID, action, target)
}

// GetPermissions returns all effective permissions (eft=allow) for the agent,
// including permissions inherited through global role assignments.
func (c *CasbinAuthorizationChecker) GetPermissions(_ context.Context, agentID string) ([]application.Permission, error) {
	policies, err := c.enforcer.GetImplicitPermissionsForUser(agentID, "*")
	if err != nil {
		return nil, fmt.Errorf("failed to get implicit permissions for %s: %w", agentID, err)
	}

	var result []application.Permission
	for _, p := range policies {
		if len(p) >= 5 && p[4] == "allow" {
			result = append(result, application.Permission{
				Assignee: p[0],
				Action:   p[2],
				Target:   p[3],
			})
		}
	}
	return result, nil
}

// GetProhibitions returns all effective prohibitions (eft=deny) for the agent,
// including prohibitions inherited through global role assignments.
func (c *CasbinAuthorizationChecker) GetProhibitions(_ context.Context, agentID string) ([]application.Permission, error) {
	policies, err := c.enforcer.GetImplicitPermissionsForUser(agentID, "*")
	if err != nil {
		return nil, fmt.Errorf("failed to get implicit permissions for %s: %w", agentID, err)
	}

	var result []application.Permission
	for _, p := range policies {
		if len(p) >= 5 && p[4] == "deny" {
			result = append(result, application.Permission{
				Assignee: p[0],
				Action:   p[2],
				Target:   p[3],
			})
		}
	}
	return result, nil
}

// AddPermission adds a global permission (allow) policy.
func (c *CasbinAuthorizationChecker) AddPermission(assignee, action, target string) error {
	_, err := c.enforcer.AddPolicy(assignee, "*", action, target, "allow")
	return err
}

// AddProhibition adds a global prohibition (deny) policy.
func (c *CasbinAuthorizationChecker) AddProhibition(assignee, action, target string) error {
	_, err := c.enforcer.AddPolicy(assignee, "*", action, target, "deny")
	return err
}

// RemovePermission removes a global permission (allow) policy.
func (c *CasbinAuthorizationChecker) RemovePermission(assignee, action, target string) error {
	_, err := c.enforcer.RemovePolicy(assignee, "*", action, target, "allow")
	return err
}

// RemoveProhibition removes a global prohibition (deny) policy.
func (c *CasbinAuthorizationChecker) RemoveProhibition(assignee, action, target string) error {
	_, err := c.enforcer.RemovePolicy(assignee, "*", action, target, "deny")
	return err
}

// AssignRole assigns a global role to an agent (applies in all domains).
func (c *CasbinAuthorizationChecker) AssignRole(agentID, roleID string) error {
	_, err := c.enforcer.AddGroupingPolicy(agentID, roleID, "*")
	return err
}

// AssignAccountRole assigns a role to an agent within a specific account.
func (c *CasbinAuthorizationChecker) AssignAccountRole(agentID, roleID, accountID string) error {
	_, err := c.enforcer.AddGroupingPolicy(agentID, roleID, accountID)
	return err
}

// RevokeRole removes a global role assignment from an agent.
func (c *CasbinAuthorizationChecker) RevokeRole(agentID, roleID string) error {
	_, err := c.enforcer.RemoveGroupingPolicy(agentID, roleID, "*")
	return err
}

// RevokeAccountRole removes an account-scoped role assignment from an agent.
func (c *CasbinAuthorizationChecker) RevokeAccountRole(agentID, roleID, accountID string) error {
	_, err := c.enforcer.RemoveGroupingPolicy(agentID, roleID, accountID)
	return err
}
