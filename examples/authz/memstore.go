package main

import (
	"context"
	"sync"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
)

// MemoryPermissionStore implements application.PermissionStore in-memory.
type MemoryPermissionStore struct {
	mu           sync.RWMutex
	permissions  map[string][]application.Permission // assigneeID -> permissions
	prohibitions map[string][]application.Permission // assigneeID -> prohibitions
	roles        map[string][]string                 // agentID -> global roleIDs
	accountRoles map[string]map[string][]string      // agentID -> accountID -> roleIDs
}

func NewMemoryPermissionStore() *MemoryPermissionStore {
	return &MemoryPermissionStore{
		permissions:  make(map[string][]application.Permission),
		prohibitions: make(map[string][]application.Permission),
		roles:        make(map[string][]string),
		accountRoles: make(map[string]map[string][]string),
	}
}

func (m *MemoryPermissionStore) GetPermissionsForAssignee(_ context.Context, assigneeID string) ([]application.Permission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.permissions[assigneeID], nil
}

func (m *MemoryPermissionStore) GetProhibitionsForAssignee(_ context.Context, assigneeID string) ([]application.Permission, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.prohibitions[assigneeID], nil
}

func (m *MemoryPermissionStore) GetRolesForAgent(_ context.Context, agentID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.roles[agentID], nil
}

func (m *MemoryPermissionStore) GetRolesForAgentInAccount(_ context.Context, agentID, accountID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if accounts, ok := m.accountRoles[agentID]; ok {
		return accounts[accountID], nil
	}
	return nil, nil
}

// AddPermission adds a permission for the given assignee.
func (m *MemoryPermissionStore) AddPermission(assignee, action, target string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.permissions[assignee] = append(m.permissions[assignee], application.Permission{
		Assignee: assignee,
		Action:   action,
		Target:   target,
	})
}

// AddProhibition adds a prohibition for the given assignee.
func (m *MemoryPermissionStore) AddProhibition(assignee, action, target string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prohibitions[assignee] = append(m.prohibitions[assignee], application.Permission{
		Assignee: assignee,
		Action:   action,
		Target:   target,
	})
}

// AssignRole assigns a global role to an agent.
func (m *MemoryPermissionStore) AssignRole(agentID, roleID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roles[agentID] = append(m.roles[agentID], roleID)
}

// AssignAccountRole assigns a role to an agent within a specific account.
func (m *MemoryPermissionStore) AssignAccountRole(agentID, roleID, accountID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.accountRoles[agentID] == nil {
		m.accountRoles[agentID] = make(map[string][]string)
	}
	m.accountRoles[agentID][accountID] = append(m.accountRoles[agentID][accountID], roleID)
}
