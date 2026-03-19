package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/application"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

// --- Mock InviteRepository ---

type mockInviteRepo struct {
	invites map[string]*entities.Invite
}

func newMockInviteRepo() *mockInviteRepo {
	return &mockInviteRepo{invites: make(map[string]*entities.Invite)}
}

func (m *mockInviteRepo) Save(_ context.Context, invite *entities.Invite) error {
	m.invites[invite.GetID()] = invite
	return nil
}

func (m *mockInviteRepo) FindByID(_ context.Context, id string) (*entities.Invite, error) {
	inv, ok := m.invites[id]
	if !ok {
		return nil, nil
	}
	return inv, nil
}

func (m *mockInviteRepo) FindByEmail(_ context.Context, email string) ([]*entities.Invite, error) {
	var result []*entities.Invite
	for _, inv := range m.invites {
		if inv.Email() == email {
			result = append(result, inv)
		}
	}
	return result, nil
}

func (m *mockInviteRepo) FindByAccount(_ context.Context, accountID string) ([]*entities.Invite, error) {
	var result []*entities.Invite
	for _, inv := range m.invites {
		if inv.AccountID() == accountID {
			result = append(result, inv)
		}
	}
	return result, nil
}

func (m *mockInviteRepo) FindPendingByEmail(_ context.Context, email string) ([]*entities.Invite, error) {
	var result []*entities.Invite
	for _, inv := range m.invites {
		if inv.Email() == email && inv.Status() == entities.InviteStatusPending {
			result = append(result, inv)
		}
	}
	return result, nil
}

// --- Mock InviteTokenService ---

type mockInviteTokenService struct {
	tokens map[string]string // inviteID -> tokenString
}

func newMockInviteTokenService() *mockInviteTokenService {
	return &mockInviteTokenService{tokens: make(map[string]string)}
}

func (m *mockInviteTokenService) IssueInviteToken(_ context.Context, inviteID string, _ time.Duration) (string, error) {
	token := "invite-token-" + inviteID
	m.tokens[inviteID] = token
	return token, nil
}

func (m *mockInviteTokenService) ValidateInviteToken(_ context.Context, tokenString string) (*application.InviteClaims, error) {
	for inviteID, tok := range m.tokens {
		if tok == tokenString {
			return &application.InviteClaims{InviteID: inviteID}, nil
		}
	}
	return nil, application.ErrTokenInvalid
}

// --- Invite test helpers ---

type inviteTestDeps struct {
	invites      *mockInviteRepo
	agents       *mockAgentRepo
	accounts     *mockAccountRepo
	credentials  *mockCredentialRepo
	tokenService *mockInviteTokenService
}

func newInviteTestService() (*application.InviteService, *inviteTestDeps) {
	deps := &inviteTestDeps{
		invites:      newMockInviteRepo(),
		agents:       newMockAgentRepo(),
		accounts:     newMockAccountRepo(),
		credentials:  newMockCredentialRepo(),
		tokenService: newMockInviteTokenService(),
	}

	svc := application.NewInviteService(
		deps.invites,
		deps.agents,
		deps.accounts,
		deps.credentials,
		deps.tokenService,
	)

	return svc, deps
}

func setupAccountWithAdmin(deps *inviteTestDeps) {
	// Create admin agent
	admin, _ := new(entities.Agent).With("admin-agent", "Admin", entities.AgentTypePerson)
	deps.agents.agents["admin-agent"] = admin

	// Create account
	account, _ := new(entities.Account).With("account-1", "Test Account", entities.AccountTypeTeam)
	deps.accounts.accounts["account-1"] = account
	deps.accounts.byMember["admin-agent"] = account
	deps.accounts.memberRoles["account-1:admin-agent"] = entities.RoleAdmin
}

// --- InviteService Tests ---

func TestInviteService_CreateInvite_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newInviteTestService()
	setupAccountWithAdmin(deps)

	invite, token, err := svc.CreateInvite(ctx, "account-1", "alice@example.com", entities.RoleMember, "admin-agent")
	if err != nil {
		t.Fatalf("CreateInvite() error: %v", err)
	}

	if invite == nil {
		t.Fatal("expected non-nil invite")
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	if invite.AccountID() != "account-1" {
		t.Errorf("AccountID() = %q, want %q", invite.AccountID(), "account-1")
	}
	if invite.Email() != "alice@example.com" {
		t.Errorf("Email() = %q, want %q", invite.Email(), "alice@example.com")
	}
	if invite.RoleID() != entities.RoleMember {
		t.Errorf("RoleID() = %q, want %q", invite.RoleID(), entities.RoleMember)
	}
	if invite.Status() != entities.InviteStatusPending {
		t.Errorf("Status() = %q, want %q", invite.Status(), entities.InviteStatusPending)
	}

	// Verify skeleton agent was created
	skeletonAgent := deps.agents.agents[invite.InviteeAgentID()]
	if skeletonAgent == nil {
		t.Fatal("expected skeleton agent to be saved")
	}
	if skeletonAgent.Status() != entities.AgentStatusInvited {
		t.Errorf("skeleton agent Status() = %q, want %q", skeletonAgent.Status(), entities.AgentStatusInvited)
	}

	// Verify invite was saved
	if len(deps.invites.invites) != 1 {
		t.Errorf("expected 1 saved invite, got %d", len(deps.invites.invites))
	}
}

func TestInviteService_CreateInvite_NotAdmin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newInviteTestService()

	// Create a non-member agent
	nonMember, _ := new(entities.Agent).With("non-member", "NonMember", entities.AgentTypePerson)
	deps.agents.agents["non-member"] = nonMember

	// Create account but don't add non-member
	account, _ := new(entities.Account).With("account-1", "Test Account", entities.AccountTypeTeam)
	deps.accounts.accounts["account-1"] = account

	_, _, err := svc.CreateInvite(ctx, "account-1", "alice@example.com", entities.RoleMember, "non-member")
	if !errors.Is(err, application.ErrNotAccountAdmin) {
		t.Fatalf("expected ErrNotAccountAdmin, got %v", err)
	}
}

func TestInviteService_CreateInvite_MemberCannotInvite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newInviteTestService()

	// Create a member agent (not admin/owner)
	member, _ := new(entities.Agent).With("member-agent", "Member", entities.AgentTypePerson)
	deps.agents.agents["member-agent"] = member

	// Create account and add member with "member" role
	account, _ := new(entities.Account).With("account-1", "Test Account", entities.AccountTypeTeam)
	deps.accounts.accounts["account-1"] = account
	deps.accounts.byMember["member-agent"] = account
	deps.accounts.memberRoles["account-1:member-agent"] = entities.RoleMember

	_, _, err := svc.CreateInvite(ctx, "account-1", "alice@example.com", entities.RoleMember, "member-agent")
	if !errors.Is(err, application.ErrNotAccountAdmin) {
		t.Fatalf("expected ErrNotAccountAdmin, got %v", err)
	}
}

func TestInviteService_CreateInvite_DuplicatePending(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newInviteTestService()
	setupAccountWithAdmin(deps)

	// First invite
	invite1, _, err := svc.CreateInvite(ctx, "account-1", "alice@example.com", entities.RoleMember, "admin-agent")
	if err != nil {
		t.Fatalf("first CreateInvite() error: %v", err)
	}

	// Second invite for same email+account should return existing
	invite2, _, err := svc.CreateInvite(ctx, "account-1", "alice@example.com", entities.RoleMember, "admin-agent")
	if err != nil {
		t.Fatalf("second CreateInvite() error: %v", err)
	}

	if invite2.GetID() != invite1.GetID() {
		t.Errorf("expected same invite ID, got %q and %q", invite1.GetID(), invite2.GetID())
	}
}

func TestInviteService_AcceptInvite_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newInviteTestService()
	setupAccountWithAdmin(deps)

	// Create invite
	invite, token, err := svc.CreateInvite(ctx, "account-1", "alice@example.com", entities.RoleMember, "admin-agent")
	if err != nil {
		t.Fatalf("CreateInvite() error: %v", err)
	}

	userInfo := application.UserInfo{
		ProviderUserID: "google-user-alice",
		Email:          "alice@example.com",
		DisplayName:    "Alice Smith",
		Provider:       "google",
	}

	agent, credential, account, err := svc.AcceptInvite(ctx, token, userInfo)
	if err != nil {
		t.Fatalf("AcceptInvite() error: %v", err)
	}

	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	if credential == nil {
		t.Fatal("expected non-nil credential")
	}
	if account == nil {
		t.Fatal("expected non-nil account")
	}

	// Agent should be activated with updated name
	if !agent.Active() {
		t.Error("expected agent to be active after acceptance")
	}
	if agent.Name() != "Alice Smith" {
		t.Errorf("agent Name() = %q, want %q", agent.Name(), "Alice Smith")
	}

	// Credential should link to the agent
	if credential.AgentID() != agent.GetID() {
		t.Errorf("credential AgentID() = %q, want %q", credential.AgentID(), agent.GetID())
	}

	// Invite should be accepted
	savedInvite := deps.invites.invites[invite.GetID()]
	if savedInvite.Status() != entities.InviteStatusAccepted {
		t.Errorf("invite Status() = %q, want %q", savedInvite.Status(), entities.InviteStatusAccepted)
	}
}

func TestInviteService_AcceptInvite_Expired(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newInviteTestService()
	setupAccountWithAdmin(deps)

	// Create an expired invite directly
	expiredInvite, _ := new(entities.Invite).With("expired-invite", "account-1", "alice@example.com", entities.RoleMember, "admin-agent", "agent-expired", time.Now().Add(-1*time.Hour))
	deps.invites.invites["expired-invite"] = expiredInvite

	// Create a skeleton agent for it
	skeletonAgent, _ := new(entities.Agent).WithInvite("agent-expired", "alice@example.com")
	deps.agents.agents["agent-expired"] = skeletonAgent

	// Register the token
	deps.tokenService.tokens["expired-invite"] = "expired-token"

	_, _, _, err := svc.AcceptInvite(ctx, "expired-token", application.UserInfo{
		ProviderUserID: "google-user-alice",
		Email:          "alice@example.com",
		DisplayName:    "Alice",
		Provider:       "google",
	})
	if !errors.Is(err, application.ErrInviteExpired) {
		t.Fatalf("expected ErrInviteExpired, got %v", err)
	}
}

func TestInviteService_AcceptInvite_AlreadyAccepted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newInviteTestService()
	setupAccountWithAdmin(deps)

	// Create and accept invite
	_, token, err := svc.CreateInvite(ctx, "account-1", "alice@example.com", entities.RoleMember, "admin-agent")
	if err != nil {
		t.Fatalf("CreateInvite() error: %v", err)
	}

	userInfo := application.UserInfo{
		ProviderUserID: "google-user-alice",
		Email:          "alice@example.com",
		DisplayName:    "Alice",
		Provider:       "google",
	}

	_, _, _, err = svc.AcceptInvite(ctx, token, userInfo)
	if err != nil {
		t.Fatalf("first AcceptInvite() error: %v", err)
	}

	// Try to accept again
	_, _, _, err = svc.AcceptInvite(ctx, token, userInfo)
	if !errors.Is(err, application.ErrInviteNotPending) {
		t.Fatalf("expected ErrInviteNotPending, got %v", err)
	}
}

func TestInviteService_RevokeInvite_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newInviteTestService()
	setupAccountWithAdmin(deps)

	invite, _, err := svc.CreateInvite(ctx, "account-1", "alice@example.com", entities.RoleMember, "admin-agent")
	if err != nil {
		t.Fatalf("CreateInvite() error: %v", err)
	}

	err = svc.RevokeInvite(ctx, invite.GetID(), "admin-agent")
	if err != nil {
		t.Fatalf("RevokeInvite() error: %v", err)
	}

	// Verify invite is revoked
	savedInvite := deps.invites.invites[invite.GetID()]
	if savedInvite.Status() != entities.InviteStatusRevoked {
		t.Errorf("invite Status() = %q, want %q", savedInvite.Status(), entities.InviteStatusRevoked)
	}

	// Verify skeleton agent is deactivated
	skeletonAgent := deps.agents.agents[invite.InviteeAgentID()]
	if skeletonAgent.Active() {
		t.Error("expected skeleton agent to be deactivated")
	}
}

func TestInviteService_RevokeInvite_AlreadyAccepted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newInviteTestService()
	setupAccountWithAdmin(deps)

	// Create and accept
	invite, token, err := svc.CreateInvite(ctx, "account-1", "alice@example.com", entities.RoleMember, "admin-agent")
	if err != nil {
		t.Fatalf("CreateInvite() error: %v", err)
	}

	_, _, _, err = svc.AcceptInvite(ctx, token, application.UserInfo{
		ProviderUserID: "google-user-alice",
		Email:          "alice@example.com",
		DisplayName:    "Alice",
		Provider:       "google",
	})
	if err != nil {
		t.Fatalf("AcceptInvite() error: %v", err)
	}

	// Try to revoke accepted invite
	err = svc.RevokeInvite(ctx, invite.GetID(), "admin-agent")
	if !errors.Is(err, application.ErrInviteNotPending) {
		t.Fatalf("expected ErrInviteNotPending, got %v", err)
	}
}

func TestInviteService_RevokeInvite_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, _ := newInviteTestService()

	err := svc.RevokeInvite(ctx, "nonexistent", "admin-agent")
	if !errors.Is(err, application.ErrInviteNotFound) {
		t.Fatalf("expected ErrInviteNotFound, got %v", err)
	}
}

func TestInviteService_AcceptInvite_EmailMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newInviteTestService()
	setupAccountWithAdmin(deps)

	// Create invite for alice
	_, token, err := svc.CreateInvite(ctx, "account-1", "alice@example.com", entities.RoleMember, "admin-agent")
	if err != nil {
		t.Fatalf("CreateInvite() error: %v", err)
	}

	// Try to accept with bob's email
	_, _, _, err = svc.AcceptInvite(ctx, token, application.UserInfo{
		ProviderUserID: "google-user-bob",
		Email:          "bob@example.com",
		DisplayName:    "Bob",
		Provider:       "google",
	})
	if !errors.Is(err, application.ErrInviteEmailMismatch) {
		t.Fatalf("expected ErrInviteEmailMismatch, got %v", err)
	}
}

func TestInviteService_AcceptInvite_InvalidToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, _ := newInviteTestService()

	_, _, _, err := svc.AcceptInvite(ctx, "invalid-token", application.UserInfo{})
	if !errors.Is(err, application.ErrInviteTokenInvalid) {
		t.Fatalf("expected ErrInviteTokenInvalid, got %v", err)
	}
}

func TestInviteService_AcceptInvite_EmptyDisplayName_KeepsEmail(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	svc, deps := newInviteTestService()
	setupAccountWithAdmin(deps)

	_, token, err := svc.CreateInvite(ctx, "account-1", "alice@example.com", entities.RoleMember, "admin-agent")
	if err != nil {
		t.Fatalf("CreateInvite() error: %v", err)
	}

	// Accept with empty DisplayName
	agent, _, _, err := svc.AcceptInvite(ctx, token, application.UserInfo{
		ProviderUserID: "google-user-alice",
		Email:          "alice@example.com",
		DisplayName:    "",
		Provider:       "google",
	})
	if err != nil {
		t.Fatalf("AcceptInvite() error: %v", err)
	}

	// Agent name should remain the email placeholder
	if agent.Name() != "alice@example.com" {
		t.Errorf("agent Name() = %q, want %q", agent.Name(), "alice@example.com")
	}
}
