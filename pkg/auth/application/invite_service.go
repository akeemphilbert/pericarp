package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
	esApplication "github.com/akeemphilbert/pericarp/pkg/eventsourcing/application"
	esDomain "github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
	"github.com/segmentio/ksuid"
)

// Sentinel errors for the invite application service.
var (
	ErrInviteNotFound      = errors.New("invite: not found")
	ErrInviteExpired       = errors.New("invite: has expired")
	ErrInviteNotPending    = errors.New("invite: not in pending status")
	ErrNotAccountAdmin     = errors.New("invite: agent is not an admin or owner of the account")
	ErrInviteeAgentMissing = errors.New("invite: invitee agent not found")
	ErrInviteTokenInvalid  = errors.New("invite: token is invalid")
	ErrInviteEmailMismatch = errors.New("invite: authenticated email does not match invite")
)

const defaultInviteExpiry = 7 * 24 * time.Hour

// InviteService orchestrates the invite flow: creating, accepting, and revoking invites.
type InviteService struct {
	invites      repositories.InviteRepository
	agents       repositories.AgentRepository
	accounts     repositories.AccountRepository
	credentials  repositories.CredentialRepository
	tokenService InviteTokenService
	eventStore   esDomain.EventStore
	logger       Logger
}

// NewInviteService creates a new InviteService with the given dependencies.
func NewInviteService(
	invites repositories.InviteRepository,
	agents repositories.AgentRepository,
	accounts repositories.AccountRepository,
	credentials repositories.CredentialRepository,
	tokenService InviteTokenService,
	opts ...InviteServiceOption,
) *InviteService {
	s := &InviteService{
		invites:      invites,
		agents:       agents,
		accounts:     accounts,
		credentials:  credentials,
		tokenService: tokenService,
		logger:       NoOpLogger{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// CreateInvite creates an invite for the given email to join an account with the specified role.
// Returns the invite and a signed invite token.
func (s *InviteService) CreateInvite(ctx context.Context, accountID, email, roleID, inviterAgentID string) (*entities.Invite, string, error) {
	// Validate the inviter is an admin/owner of the account
	if err := s.validateAdminOrOwner(ctx, accountID, inviterAgentID); err != nil {
		return nil, "", err
	}

	// Check for existing pending invite for same email+account (idempotency)
	pending, err := s.invites.FindPendingByEmail(ctx, email)
	if err != nil {
		return nil, "", fmt.Errorf("invite: failed to check pending invites: %w", err)
	}
	for _, p := range pending {
		if p.AccountID() == accountID {
			// Return existing pending invite with a fresh token
			token, tokenErr := s.tokenService.IssueInviteToken(ctx, p.GetID(), time.Until(p.ExpiresAt()))
			if tokenErr != nil {
				return nil, "", fmt.Errorf("invite: failed to issue token for existing invite: %w", tokenErr)
			}
			return p, token, nil
		}
	}

	// Create skeleton agent via WithInvite
	agentID := ksuid.New().String()
	agent, err := new(entities.Agent).WithInvite(agentID, email)
	if err != nil {
		return nil, "", fmt.Errorf("invite: failed to create skeleton agent: %w", err)
	}

	// Create Invite aggregate
	inviteID := ksuid.New().String()
	expiresAt := time.Now().Add(defaultInviteExpiry)
	invite, err := new(entities.Invite).With(inviteID, accountID, email, roleID, inviterAgentID, agentID, expiresAt)
	if err != nil {
		return nil, "", fmt.Errorf("invite: failed to create invite: %w", err)
	}

	// Commit events atomically via UnitOfWork
	if s.eventStore != nil {
		uow := esApplication.NewSimpleUnitOfWork(s.eventStore, nil)
		if err = uow.Track(agent, invite); err != nil {
			return nil, "", fmt.Errorf("invite: failed to track entities: %w", err)
		}
		if err = uow.Commit(ctx); err != nil {
			return nil, "", fmt.Errorf("invite: failed to commit unit of work: %w", err)
		}
	}

	// Save projections
	if err = s.agents.Save(ctx, agent); err != nil {
		return nil, "", fmt.Errorf("invite: failed to save agent: %w", err)
	}
	if err = s.invites.Save(ctx, invite); err != nil {
		return nil, "", fmt.Errorf("invite: failed to save invite: %w", err)
	}

	// Issue invite token
	token, err := s.tokenService.IssueInviteToken(ctx, inviteID, defaultInviteExpiry)
	if err != nil {
		return nil, "", fmt.Errorf("invite: failed to issue invite token: %w", err)
	}

	s.logger.Info(ctx, "invite created",
		"invite_id", inviteID,
		"account_id", accountID,
		"email", email,
		"inviter", inviterAgentID,
	)

	return invite, token, nil
}

// AcceptInvite accepts an invite using the provided token and user info.
// Returns the activated agent, credential, and account.
func (s *InviteService) AcceptInvite(ctx context.Context, token string, userInfo UserInfo) (*entities.Agent, *entities.Credential, *entities.Account, error) {
	// Validate invite token
	claims, err := s.tokenService.ValidateInviteToken(ctx, token)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("%w: %v", ErrInviteTokenInvalid, err)
	}

	// Load invite
	invite, err := s.invites.FindByID(ctx, claims.InviteID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invite: failed to find invite: %w", err)
	}
	if invite == nil {
		return nil, nil, nil, ErrInviteNotFound
	}
	if invite.Status() != entities.InviteStatusPending {
		return nil, nil, nil, ErrInviteNotPending
	}
	if invite.IsExpired() {
		return nil, nil, nil, ErrInviteExpired
	}

	// Verify the authenticated user's email matches the invite recipient
	if userInfo.Email != invite.Email() {
		return nil, nil, nil, ErrInviteEmailMismatch
	}

	// Load skeleton agent
	agent, err := s.agents.FindByID(ctx, invite.InviteeAgentID())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invite: failed to find invitee agent: %w", err)
	}
	if agent == nil {
		return nil, nil, nil, ErrInviteeAgentMissing
	}

	// Activate agent and update name from userInfo
	if userInfo.DisplayName != "" {
		if err = agent.UpdateName(userInfo.DisplayName); err != nil {
			return nil, nil, nil, fmt.Errorf("invite: failed to update agent name: %w", err)
		}
	}
	if err = agent.Activate(); err != nil {
		return nil, nil, nil, fmt.Errorf("invite: failed to activate agent: %w", err)
	}

	// Create credential for the agent
	credentialID := ksuid.New().String()
	credential, err := new(entities.Credential).With(
		credentialID, agent.GetID(),
		userInfo.Provider, userInfo.ProviderUserID,
		userInfo.Email, userInfo.DisplayName,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invite: failed to create credential: %w", err)
	}

	// Load account and add member with pre-assigned role
	account, err := s.accounts.FindByID(ctx, invite.AccountID())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invite: failed to find account: %w", err)
	}
	if account == nil {
		return nil, nil, nil, fmt.Errorf("invite: account %s not found", invite.AccountID())
	}

	if err = account.AddMember(agent.GetID(), invite.RoleID()); err != nil {
		return nil, nil, nil, fmt.Errorf("invite: failed to add member to account: %w", err)
	}

	// Mark invite as accepted
	if err = invite.Accept(); err != nil {
		return nil, nil, nil, fmt.Errorf("invite: failed to accept invite: %w", err)
	}

	// Commit events atomically
	if s.eventStore != nil {
		uow := esApplication.NewSimpleUnitOfWork(s.eventStore, nil)
		if err = uow.Track(agent, credential, account, invite); err != nil {
			return nil, nil, nil, fmt.Errorf("invite: failed to track entities: %w", err)
		}
		if err = uow.Commit(ctx); err != nil {
			return nil, nil, nil, fmt.Errorf("invite: failed to commit unit of work: %w", err)
		}
	}

	// Save projections
	if err = s.agents.Save(ctx, agent); err != nil {
		return nil, nil, nil, fmt.Errorf("invite: failed to save agent: %w", err)
	}
	if err = s.credentials.Save(ctx, credential); err != nil {
		return nil, nil, nil, fmt.Errorf("invite: failed to save credential: %w", err)
	}
	if err = s.accounts.Save(ctx, account); err != nil {
		return nil, nil, nil, fmt.Errorf("invite: failed to save account: %w", err)
	}
	if err = s.accounts.SaveMember(ctx, invite.AccountID(), agent.GetID(), invite.RoleID()); err != nil {
		return nil, nil, nil, fmt.Errorf("invite: failed to save account member: %w", err)
	}
	if err = s.invites.Save(ctx, invite); err != nil {
		return nil, nil, nil, fmt.Errorf("invite: failed to save invite: %w", err)
	}

	s.logger.Info(ctx, "invite accepted",
		"invite_id", invite.GetID(),
		"agent_id", agent.GetID(),
		"account_id", invite.AccountID(),
	)

	return agent, credential, account, nil
}

// RevokeInvite revokes a pending invite.
func (s *InviteService) RevokeInvite(ctx context.Context, inviteID, revokerAgentID string) error {
	invite, err := s.invites.FindByID(ctx, inviteID)
	if err != nil {
		return fmt.Errorf("invite: failed to find invite: %w", err)
	}
	if invite == nil {
		return ErrInviteNotFound
	}

	// Validate revoker is admin/owner
	if err = s.validateAdminOrOwner(ctx, invite.AccountID(), revokerAgentID); err != nil {
		return err
	}

	if invite.Status() != entities.InviteStatusPending {
		return ErrInviteNotPending
	}

	// Revoke the invite
	if err = invite.Revoke(); err != nil {
		return fmt.Errorf("invite: failed to revoke invite: %w", err)
	}

	// Deactivate skeleton agent
	agent, err := s.agents.FindByID(ctx, invite.InviteeAgentID())
	if err != nil {
		return fmt.Errorf("invite: failed to find invitee agent: %w", err)
	}
	if agent != nil {
		if err = agent.Deactivate(); err != nil {
			return fmt.Errorf("invite: failed to deactivate agent: %w", err)
		}
	} else {
		s.logger.Warn(ctx, "skeleton agent not found during invite revocation",
			"invite_id", inviteID,
			"invitee_agent_id", invite.InviteeAgentID(),
		)
	}

	// Commit events atomically
	if s.eventStore != nil {
		uow := esApplication.NewSimpleUnitOfWork(s.eventStore, nil)
		if agent != nil {
			if err = uow.Track(agent, invite); err != nil {
				return fmt.Errorf("invite: failed to track entities: %w", err)
			}
		} else {
			if err = uow.Track(invite); err != nil {
				return fmt.Errorf("invite: failed to track entities: %w", err)
			}
		}
		if err = uow.Commit(ctx); err != nil {
			return fmt.Errorf("invite: failed to commit unit of work: %w", err)
		}
	}

	if agent != nil {
		if err = s.agents.Save(ctx, agent); err != nil {
			return fmt.Errorf("invite: failed to save agent: %w", err)
		}
	}

	if err = s.invites.Save(ctx, invite); err != nil {
		return fmt.Errorf("invite: failed to save invite: %w", err)
	}

	s.logger.Info(ctx, "invite revoked",
		"invite_id", inviteID,
		"revoker", revokerAgentID,
	)

	return nil
}

// validateAdminOrOwner checks that the given agent is an admin or owner of the account.
func (s *InviteService) validateAdminOrOwner(ctx context.Context, accountID, agentID string) error {
	roleID, err := s.accounts.FindMemberRole(ctx, accountID, agentID)
	if err != nil {
		return fmt.Errorf("invite: failed to find member role: %w", err)
	}
	if roleID == entities.RoleOwner || roleID == entities.RoleAdmin {
		return nil
	}
	return ErrNotAccountAdmin
}
