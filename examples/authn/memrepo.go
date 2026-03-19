package main

import (
	"context"
	"sync"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
)

// MemoryAgentRepository implements repositories.AgentRepository in-memory.
type MemoryAgentRepository struct {
	mu     sync.RWMutex
	agents map[string]*entities.Agent
}

func NewMemoryAgentRepository() *MemoryAgentRepository {
	return &MemoryAgentRepository{agents: make(map[string]*entities.Agent)}
}

func (m *MemoryAgentRepository) Save(_ context.Context, agent *entities.Agent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agents[agent.GetID()] = agent
	return nil
}

func (m *MemoryAgentRepository) FindByID(_ context.Context, id string) (*entities.Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	agent, ok := m.agents[id]
	if !ok {
		return nil, nil
	}
	return agent, nil
}

func (m *MemoryAgentRepository) FindAll(_ context.Context, _ string, _ int) (*repositories.PaginatedResponse[*entities.Agent], error) {
	return nil, nil
}

// MemoryCredentialRepository implements repositories.CredentialRepository in-memory.
type MemoryCredentialRepository struct {
	mu          sync.RWMutex
	credentials map[string]*entities.Credential
	byProvider  map[string]*entities.Credential // key: "provider:providerUserID"
}

func NewMemoryCredentialRepository() *MemoryCredentialRepository {
	return &MemoryCredentialRepository{
		credentials: make(map[string]*entities.Credential),
		byProvider:  make(map[string]*entities.Credential),
	}
}

func (m *MemoryCredentialRepository) Save(_ context.Context, credential *entities.Credential) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.credentials[credential.GetID()] = credential
	m.byProvider[credential.Provider()+":"+credential.ProviderUserID()] = credential
	return nil
}

func (m *MemoryCredentialRepository) FindByID(_ context.Context, id string) (*entities.Credential, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cred, ok := m.credentials[id]
	if !ok {
		return nil, nil
	}
	return cred, nil
}

func (m *MemoryCredentialRepository) FindByProvider(_ context.Context, provider, providerUserID string) (*entities.Credential, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cred, ok := m.byProvider[provider+":"+providerUserID]
	if !ok {
		return nil, nil
	}
	return cred, nil
}

func (m *MemoryCredentialRepository) FindByEmail(_ context.Context, email string) ([]*entities.Credential, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*entities.Credential
	for _, cred := range m.credentials {
		if cred.Email() == email {
			result = append(result, cred)
		}
	}
	return result, nil
}

func (m *MemoryCredentialRepository) FindByAgent(_ context.Context, agentID string) ([]*entities.Credential, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*entities.Credential
	for _, cred := range m.credentials {
		if cred.AgentID() == agentID {
			result = append(result, cred)
		}
	}
	return result, nil
}

func (m *MemoryCredentialRepository) FindAll(_ context.Context, _ string, _ int) (*repositories.PaginatedResponse[*entities.Credential], error) {
	return nil, nil
}

// MemorySessionRepository implements repositories.AuthSessionRepository in-memory.
type MemorySessionRepository struct {
	mu       sync.RWMutex
	sessions map[string]*entities.AuthSession
}

func NewMemorySessionRepository() *MemorySessionRepository {
	return &MemorySessionRepository{sessions: make(map[string]*entities.AuthSession)}
}

func (m *MemorySessionRepository) Save(_ context.Context, session *entities.AuthSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.GetID()] = session
	return nil
}

func (m *MemorySessionRepository) FindByID(_ context.Context, id string) (*entities.AuthSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[id]
	if !ok {
		return nil, nil
	}
	return session, nil
}

func (m *MemorySessionRepository) FindByAgent(_ context.Context, agentID string) ([]*entities.AuthSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*entities.AuthSession
	for _, sess := range m.sessions {
		if sess.AgentID() == agentID {
			result = append(result, sess)
		}
	}
	return result, nil
}

func (m *MemorySessionRepository) FindActive(_ context.Context, agentID string) ([]*entities.AuthSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []*entities.AuthSession
	for _, sess := range m.sessions {
		if sess.AgentID() == agentID && sess.Active() && !sess.IsExpired() {
			result = append(result, sess)
		}
	}
	return result, nil
}

func (m *MemorySessionRepository) FindAll(_ context.Context, _ string, _ int) (*repositories.PaginatedResponse[*entities.AuthSession], error) {
	return nil, nil
}

func (m *MemorySessionRepository) RevokeAllForAgent(_ context.Context, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sess := range m.sessions {
		if sess.AgentID() == agentID && sess.Active() {
			_ = sess.Revoke()
		}
	}
	return nil
}

// MemoryAccountRepository implements repositories.AccountRepository in-memory.
type MemoryAccountRepository struct {
	mu             sync.RWMutex
	accounts       map[string]*entities.Account
	memberAccounts map[string][]*entities.Account // agentID -> accounts
	memberRoles    map[string]string              // "accountID:agentID" -> roleID
}

func NewMemoryAccountRepository() *MemoryAccountRepository {
	return &MemoryAccountRepository{
		accounts:       make(map[string]*entities.Account),
		memberAccounts: make(map[string][]*entities.Account),
		memberRoles:    make(map[string]string),
	}
}

func (m *MemoryAccountRepository) Save(_ context.Context, account *entities.Account) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accounts[account.GetID()] = account
	return nil
}

func (m *MemoryAccountRepository) FindByID(_ context.Context, id string) (*entities.Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	account, ok := m.accounts[id]
	if !ok {
		return nil, nil
	}
	return account, nil
}

func (m *MemoryAccountRepository) FindByMember(_ context.Context, agentID string) ([]*entities.Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.memberAccounts[agentID], nil
}

func (m *MemoryAccountRepository) FindPersonalByMember(_ context.Context, agentID string) (*entities.Account, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, account := range m.memberAccounts[agentID] {
		if account.AccountType() == entities.AccountTypePersonal {
			return account, nil
		}
	}
	return nil, nil
}

func (m *MemoryAccountRepository) FindMemberRole(_ context.Context, accountID, agentID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	role := m.memberRoles[accountID+":"+agentID]
	return role, nil
}

func (m *MemoryAccountRepository) SaveMember(_ context.Context, accountID, agentID, roleID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.memberRoles[accountID+":"+agentID] = roleID
	if account, ok := m.accounts[accountID]; ok {
		// Check if already linked
		for _, a := range m.memberAccounts[agentID] {
			if a.GetID() == accountID {
				return nil
			}
		}
		m.memberAccounts[agentID] = append(m.memberAccounts[agentID], account)
	}
	return nil
}

func (m *MemoryAccountRepository) FindAll(_ context.Context, _ string, _ int) (*repositories.PaginatedResponse[*entities.Account], error) {
	return nil, nil
}
