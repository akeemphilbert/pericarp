package entities_test

import (
	"context"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

func TestAgent_With(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		id        string
		agentName string
		agentType string
		wantErr   bool
		wantType  string
	}{
		{
			name:      "creates person agent",
			id:        "agent-1",
			agentName: "Alice",
			agentType: entities.AgentTypePerson,
			wantType:  entities.AgentTypePerson,
		},
		{
			name:      "creates organization agent",
			id:        "agent-2",
			agentName: "Acme Corp",
			agentType: entities.AgentTypeOrganization,
			wantType:  entities.AgentTypeOrganization,
		},
		{
			name:      "defaults to person when type is empty",
			id:        "agent-3",
			agentName: "Bob",
			agentType: "",
			wantType:  entities.AgentTypePerson,
		},
		{
			name:      "fails with empty ID",
			id:        "",
			agentName: "Charlie",
			agentType: entities.AgentTypePerson,
			wantErr:   true,
		},
		{
			name:      "fails with empty name",
			id:        "agent-4",
			agentName: "",
			agentType: entities.AgentTypePerson,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			agent, err := new(entities.Agent).With(tt.id, tt.agentName, tt.agentType)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if agent.GetID() != tt.id {
				t.Errorf("GetID() = %q, want %q", agent.GetID(), tt.id)
			}
			if agent.Name() != tt.agentName {
				t.Errorf("Name() = %q, want %q", agent.Name(), tt.agentName)
			}
			if agent.AgentType() != tt.wantType {
				t.Errorf("AgentType() = %q, want %q", agent.AgentType(), tt.wantType)
			}
			if !agent.Active() {
				t.Error("expected agent to be active")
			}
			if agent.CreatedAt().IsZero() {
				t.Error("expected non-zero CreatedAt")
			}

			events := agent.GetUncommittedEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 uncommitted event, got %d", len(events))
			}
			if events[0].EventType != entities.EventTypeAgentCreated {
				t.Errorf("event type = %q, want %q", events[0].EventType, entities.EventTypeAgentCreated)
			}
		})
	}
}

func TestAgent_DeactivateActivate(t *testing.T) {
	t.Parallel()

	agent, err := new(entities.Agent).With("agent-1", "Alice", entities.AgentTypePerson)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Deactivate
	if err := agent.Deactivate(); err != nil {
		t.Fatalf("Deactivate() error: %v", err)
	}
	if agent.Active() {
		t.Error("expected agent to be inactive after Deactivate")
	}

	// Deactivate again (no-op)
	eventsBefore := len(agent.GetUncommittedEvents())
	if err := agent.Deactivate(); err != nil {
		t.Fatalf("Deactivate() error: %v", err)
	}
	if len(agent.GetUncommittedEvents()) != eventsBefore {
		t.Error("expected no new event from duplicate Deactivate")
	}

	// Activate
	if err := agent.Activate(); err != nil {
		t.Fatalf("Activate() error: %v", err)
	}
	if !agent.Active() {
		t.Error("expected agent to be active after Activate")
	}

	// Activate again (no-op)
	eventsBefore = len(agent.GetUncommittedEvents())
	if err := agent.Activate(); err != nil {
		t.Fatalf("Activate() error: %v", err)
	}
	if len(agent.GetUncommittedEvents()) != eventsBefore {
		t.Error("expected no new event from duplicate Activate")
	}
}

func TestAgent_AssignRole(t *testing.T) {
	t.Parallel()

	agent, err := new(entities.Agent).With("agent-1", "Alice", entities.AgentTypePerson)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := agent.AssignRole("role-admin"); err != nil {
		t.Fatalf("AssignRole() error: %v", err)
	}

	events := agent.GetUncommittedEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	lastEvent := events[1]
	if lastEvent.EventType != entities.EventTypeAgentRoleAssigned {
		t.Errorf("event type = %q, want %q", lastEvent.EventType, entities.EventTypeAgentRoleAssigned)
	}

	payload, ok := lastEvent.Payload.(entities.AgentRoleAssigned)
	if !ok {
		t.Fatalf("expected AgentRoleAssigned payload, got %T", lastEvent.Payload)
	}
	if payload.Subject != "agent-1" {
		t.Errorf("Subject = %q, want %q", payload.Subject, "agent-1")
	}
	if payload.Predicate != entities.PredicateHasRole {
		t.Errorf("Predicate = %q, want %q", payload.Predicate, entities.PredicateHasRole)
	}
	if payload.Object != "role-admin" {
		t.Errorf("Object = %q, want %q", payload.Object, "role-admin")
	}
}

func TestAgent_RevokeRole(t *testing.T) {
	t.Parallel()

	agent, err := new(entities.Agent).With("agent-1", "Alice", entities.AgentTypePerson)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := agent.RevokeRole("role-admin"); err != nil {
		t.Fatalf("RevokeRole() error: %v", err)
	}

	events := agent.GetUncommittedEvents()
	lastEvent := events[len(events)-1]
	if lastEvent.EventType != entities.EventTypeAgentRoleRevoked {
		t.Errorf("event type = %q, want %q", lastEvent.EventType, entities.EventTypeAgentRoleRevoked)
	}

	payload, ok := lastEvent.Payload.(entities.AgentRoleRevoked)
	if !ok {
		t.Fatalf("expected AgentRoleRevoked payload, got %T", lastEvent.Payload)
	}
	if payload.Predicate != entities.PredicateHadRole {
		t.Errorf("Predicate = %q, want %q", payload.Predicate, entities.PredicateHadRole)
	}
}

func TestAgent_GroupMembership(t *testing.T) {
	t.Parallel()

	agent, err := new(entities.Agent).With("agent-1", "Alice", entities.AgentTypePerson)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Add to group
	if err := agent.AddToGroup("group-eng"); err != nil {
		t.Fatalf("AddToGroup() error: %v", err)
	}

	events := agent.GetUncommittedEvents()
	addEvent := events[len(events)-1]
	if addEvent.EventType != entities.EventTypeAgentGroupMembershipAdded {
		t.Errorf("event type = %q, want %q", addEvent.EventType, entities.EventTypeAgentGroupMembershipAdded)
	}

	payload, ok := addEvent.Payload.(entities.AgentGroupMembershipAdded)
	if !ok {
		t.Fatalf("expected AgentGroupMembershipAdded payload, got %T", addEvent.Payload)
	}
	if payload.Predicate != entities.PredicateMember {
		t.Errorf("Predicate = %q, want %q", payload.Predicate, entities.PredicateMember)
	}

	// Remove from group
	if err := agent.RemoveFromGroup("group-eng"); err != nil {
		t.Fatalf("RemoveFromGroup() error: %v", err)
	}

	events = agent.GetUncommittedEvents()
	removeEvent := events[len(events)-1]
	if removeEvent.EventType != entities.EventTypeAgentGroupMembershipRemoved {
		t.Errorf("event type = %q, want %q", removeEvent.EventType, entities.EventTypeAgentGroupMembershipRemoved)
	}
}

func TestAgent_ValidationErrors(t *testing.T) {
	t.Parallel()

	agent, err := new(entities.Agent).With("agent-1", "Alice", entities.AgentTypePerson)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := agent.AssignRole(""); err == nil {
		t.Error("expected error for empty role ID")
	}
	if err := agent.RevokeRole(""); err == nil {
		t.Error("expected error for empty role ID")
	}
	if err := agent.AddToGroup(""); err == nil {
		t.Error("expected error for empty group ID")
	}
	if err := agent.RemoveFromGroup(""); err == nil {
		t.Error("expected error for empty group ID")
	}
}

func TestAgent_ApplyEvent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Create agent
	agent, err := new(entities.Agent).With("agent-1", "Alice", entities.AgentTypePerson)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Capture events
	events := agent.GetUncommittedEvents()
	agent.ClearUncommittedEvents()

	// Reconstruct from events
	restored := &entities.Agent{}
	if err := restored.Restore("agent-1", "placeholder", entities.AgentTypePerson, true, events[0].Created); err != nil {
		t.Fatalf("Restore() error: %v", err)
	}

	// Apply creation event
	if err := restored.ApplyEvent(ctx, events[0]); err != nil {
		t.Fatalf("ApplyEvent() error: %v", err)
	}
	if restored.Name() != "Alice" {
		t.Errorf("Name() = %q, want %q", restored.Name(), "Alice")
	}
}
