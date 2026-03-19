package auth

import (
	"context"
	"errors"
	"fmt"
)

// ErrNoIdentity is returned when the context has no Identity or the Identity
// is missing required fields (AgentID, ActiveAccountID).
var ErrNoIdentity = errors.New("no identity in context")

// ErrAccountMismatch is returned when the caller's active account does not
// match the resource's account, indicating a tenant isolation violation.
var ErrAccountMismatch = errors.New("account mismatch")

// ResourceOwnership carries the tenant and creator for a new resource.
// Aggregate constructors accept these values to tag resources at creation time.
type ResourceOwnership struct {
	AccountID        string
	CreatedByAgentID string
}

// ResourceOwnershipFromCtx extracts tenant ownership information from the
// authenticated identity in ctx. It returns ErrNoIdentity if the context has
// no identity or if AgentID or ActiveAccountID is empty.
func ResourceOwnershipFromCtx(ctx context.Context) (ResourceOwnership, error) {
	id := AgentFromCtx(ctx)
	if id == nil {
		return ResourceOwnership{}, ErrNoIdentity
	}
	if id.AgentID == "" {
		return ResourceOwnership{}, fmt.Errorf("empty AgentID: %w", ErrNoIdentity)
	}
	if id.ActiveAccountID == "" {
		return ResourceOwnership{}, fmt.Errorf("empty ActiveAccountID: %w", ErrNoIdentity)
	}
	return ResourceOwnership{
		AccountID:        id.ActiveAccountID,
		CreatedByAgentID: id.AgentID,
	}, nil
}

// VerifyAccountAccess checks that the caller's active account matches the
// given resourceAccountID. It returns ErrNoIdentity if no identity is in
// the context, or ErrAccountMismatch if the accounts differ.
func VerifyAccountAccess(ctx context.Context, resourceAccountID string) error {
	id := AgentFromCtx(ctx)
	if id == nil {
		return ErrNoIdentity
	}
	if id.ActiveAccountID != resourceAccountID {
		return ErrAccountMismatch
	}
	return nil
}
