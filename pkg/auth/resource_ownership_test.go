package auth

import (
	"context"
	"errors"
	"testing"
)

func TestResourceOwnershipFromCtx_HappyPath(t *testing.T) {
	t.Parallel()

	ctx := ContextWithAgent(context.Background(), &Identity{
		AgentID:         "agent-1",
		AccountIDs:      []string{"acc-1"},
		ActiveAccountID: "acc-1",
	})

	ownership, err := ResourceOwnershipFromCtx(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ownership.AccountID != "acc-1" {
		t.Errorf("AccountID = %q, want %q", ownership.AccountID, "acc-1")
	}
	if ownership.CreatedByAgentID != "agent-1" {
		t.Errorf("CreatedByAgentID = %q, want %q", ownership.CreatedByAgentID, "agent-1")
	}
}

func TestResourceOwnershipFromCtx_NoIdentity(t *testing.T) {
	t.Parallel()

	_, err := ResourceOwnershipFromCtx(context.Background())
	if !errors.Is(err, ErrNoIdentity) {
		t.Errorf("error = %v, want %v", err, ErrNoIdentity)
	}
}

func TestResourceOwnershipFromCtx_EmptyAgentID(t *testing.T) {
	t.Parallel()

	ctx := ContextWithAgent(context.Background(), &Identity{
		AgentID:         "",
		ActiveAccountID: "acc-1",
	})

	_, err := ResourceOwnershipFromCtx(ctx)
	if !errors.Is(err, ErrNoIdentity) {
		t.Errorf("error = %v, want wrapping %v", err, ErrNoIdentity)
	}
}

func TestResourceOwnershipFromCtx_EmptyActiveAccountID(t *testing.T) {
	t.Parallel()

	ctx := ContextWithAgent(context.Background(), &Identity{
		AgentID:         "agent-1",
		ActiveAccountID: "",
	})

	_, err := ResourceOwnershipFromCtx(ctx)
	if !errors.Is(err, ErrNoIdentity) {
		t.Errorf("error = %v, want wrapping %v", err, ErrNoIdentity)
	}
}

func TestVerifyAccountAccess_Matching(t *testing.T) {
	t.Parallel()

	ctx := ContextWithAgent(context.Background(), &Identity{
		AgentID:         "agent-1",
		ActiveAccountID: "acc-1",
	})

	if err := VerifyAccountAccess(ctx, "acc-1"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyAccountAccess_Mismatch(t *testing.T) {
	t.Parallel()

	ctx := ContextWithAgent(context.Background(), &Identity{
		AgentID:         "agent-1",
		ActiveAccountID: "acc-1",
	})

	err := VerifyAccountAccess(ctx, "acc-other")
	if !errors.Is(err, ErrAccountMismatch) {
		t.Errorf("error = %v, want %v", err, ErrAccountMismatch)
	}
}

func TestVerifyAccountAccess_NoIdentity(t *testing.T) {
	t.Parallel()

	err := VerifyAccountAccess(context.Background(), "acc-1")
	if !errors.Is(err, ErrNoIdentity) {
		t.Errorf("error = %v, want %v", err, ErrNoIdentity)
	}
}
