package auth

import (
	"context"
	"testing"
)

func TestAgentFromCtx_NoIdentity_ReturnsNil(t *testing.T) {
	t.Parallel()

	id := AgentFromCtx(context.Background())
	if id != nil {
		t.Errorf("expected nil Identity from empty context, got %v", id)
	}
}

func TestContextWithAgent_RoundTrip(t *testing.T) {
	t.Parallel()

	want := &Identity{
		AgentID:         "agent-1",
		AccountIDs:      []string{"acc-1", "acc-2"},
		ActiveAccountID: "acc-1",
	}

	ctx := ContextWithAgent(context.Background(), want)
	got := AgentFromCtx(ctx)

	if got == nil {
		t.Fatal("expected Identity in context, got nil")
	}
	if got.AgentID != want.AgentID {
		t.Errorf("AgentID = %q, want %q", got.AgentID, want.AgentID)
	}
	if got.ActiveAccountID != want.ActiveAccountID {
		t.Errorf("ActiveAccountID = %q, want %q", got.ActiveAccountID, want.ActiveAccountID)
	}
	if len(got.AccountIDs) != len(want.AccountIDs) {
		t.Fatalf("AccountIDs length = %d, want %d", len(got.AccountIDs), len(want.AccountIDs))
	}
	for i, id := range got.AccountIDs {
		if id != want.AccountIDs[i] {
			t.Errorf("AccountIDs[%d] = %q, want %q", i, id, want.AccountIDs[i])
		}
	}
}

func TestAgentFromCtx_WrongType_ReturnsNil(t *testing.T) {
	t.Parallel()

	// Store a string at the identity key — AgentFromCtx should return nil, not panic.
	ctx := context.WithValue(context.Background(), identityKey, "not-an-identity")
	id := AgentFromCtx(ctx)
	if id != nil {
		t.Errorf("expected nil Identity for wrong type, got %v", id)
	}
}
