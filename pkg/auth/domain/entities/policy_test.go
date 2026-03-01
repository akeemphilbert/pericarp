package entities_test

import (
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

func TestPolicy_With(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		id         string
		policyName string
		policyType string
		wantErr    bool
		wantType   string
	}{
		{
			name:       "creates set policy",
			id:         "policy-1",
			policyName: "Default Access",
			policyType: entities.PolicyTypeSet,
			wantType:   entities.PolicyTypeSet,
		},
		{
			name:       "creates agreement policy",
			id:         "policy-2",
			policyName: "Service Agreement",
			policyType: entities.PolicyTypeAgreement,
			wantType:   entities.PolicyTypeAgreement,
		},
		{
			name:       "defaults to set when type is empty",
			id:         "policy-3",
			policyName: "Unnamed Policy",
			policyType: "",
			wantType:   entities.PolicyTypeSet,
		},
		{
			name:       "fails with empty ID",
			id:         "",
			policyName: "Some Policy",
			policyType: entities.PolicyTypeSet,
			wantErr:    true,
		},
		{
			name:       "fails with empty name",
			id:         "policy-4",
			policyName: "",
			policyType: entities.PolicyTypeSet,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			policy, err := new(entities.Policy).With(tt.id, tt.policyName, tt.policyType)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if policy.GetID() != tt.id {
				t.Errorf("GetID() = %q, want %q", policy.GetID(), tt.id)
			}
			if policy.Name() != tt.policyName {
				t.Errorf("Name() = %q, want %q", policy.Name(), tt.policyName)
			}
			if policy.PolicyType() != tt.wantType {
				t.Errorf("PolicyType() = %q, want %q", policy.PolicyType(), tt.wantType)
			}
			if !policy.Active() {
				t.Error("expected policy to be active")
			}

			events := policy.GetUncommittedEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 uncommitted event, got %d", len(events))
			}
			if events[0].EventType != entities.EventTypePolicyCreated {
				t.Errorf("event type = %q, want %q", events[0].EventType, entities.EventTypePolicyCreated)
			}
		})
	}
}

func TestPolicy_ActivateDeactivate(t *testing.T) {
	t.Parallel()

	policy, err := new(entities.Policy).With("policy-1", "Test Policy", entities.PolicyTypeSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Deactivate
	if err := policy.Deactivate(); err != nil {
		t.Fatalf("Deactivate() error: %v", err)
	}
	if policy.Active() {
		t.Error("expected policy to be inactive")
	}

	// Activate
	if err := policy.Activate(); err != nil {
		t.Fatalf("Activate() error: %v", err)
	}
	if !policy.Active() {
		t.Error("expected policy to be active")
	}
}

func TestPolicy_GrantPermission(t *testing.T) {
	t.Parallel()

	policy, err := new(entities.Policy).With("policy-1", "Test Policy", entities.PolicyTypeSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := policy.GrantPermission("agent-1", entities.ActionRead, "resource-1"); err != nil {
		t.Fatalf("GrantPermission() error: %v", err)
	}

	events := policy.GetUncommittedEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	permEvent := events[1]
	if permEvent.EventType != entities.EventTypePermissionGranted {
		t.Errorf("event type = %q, want %q", permEvent.EventType, entities.EventTypePermissionGranted)
	}

	payload, ok := permEvent.Payload.(entities.PermissionGranted)
	if !ok {
		t.Fatalf("expected PermissionGranted payload, got %T", permEvent.Payload)
	}
	if payload.Subject != "agent-1" {
		t.Errorf("Subject = %q, want %q", payload.Subject, "agent-1")
	}
	if payload.Predicate != entities.PredicatePermission {
		t.Errorf("Predicate = %q, want %q", payload.Predicate, entities.PredicatePermission)
	}
	if payload.Object != "resource-1" {
		t.Errorf("Object = %q, want %q", payload.Object, "resource-1")
	}
	if payload.Action != entities.ActionRead {
		t.Errorf("Action = %q, want %q", payload.Action, entities.ActionRead)
	}
}

func TestPolicy_SetProhibition(t *testing.T) {
	t.Parallel()

	policy, err := new(entities.Policy).With("policy-1", "Test Policy", entities.PolicyTypeSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := policy.SetProhibition("agent-1", entities.ActionDelete, "resource-1"); err != nil {
		t.Fatalf("SetProhibition() error: %v", err)
	}

	events := policy.GetUncommittedEvents()
	lastEvent := events[len(events)-1]
	if lastEvent.EventType != entities.EventTypeProhibitionSet {
		t.Errorf("event type = %q, want %q", lastEvent.EventType, entities.EventTypeProhibitionSet)
	}

	payload, ok := lastEvent.Payload.(entities.ProhibitionSet)
	if !ok {
		t.Fatalf("expected ProhibitionSet payload, got %T", lastEvent.Payload)
	}
	if payload.Predicate != entities.PredicateProhibition {
		t.Errorf("Predicate = %q, want %q", payload.Predicate, entities.PredicateProhibition)
	}
	if payload.Action != entities.ActionDelete {
		t.Errorf("Action = %q, want %q", payload.Action, entities.ActionDelete)
	}
}

func TestPolicy_ImposeDuty(t *testing.T) {
	t.Parallel()

	policy, err := new(entities.Policy).With("policy-1", "Test Policy", entities.PolicyTypeSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := policy.ImposeDuty("agent-1", entities.ActionRead, "terms-of-service"); err != nil {
		t.Fatalf("ImposeDuty() error: %v", err)
	}

	events := policy.GetUncommittedEvents()
	lastEvent := events[len(events)-1]
	if lastEvent.EventType != entities.EventTypeDutyImposed {
		t.Errorf("event type = %q, want %q", lastEvent.EventType, entities.EventTypeDutyImposed)
	}

	payload, ok := lastEvent.Payload.(entities.DutyImposed)
	if !ok {
		t.Fatalf("expected DutyImposed payload, got %T", lastEvent.Payload)
	}
	if payload.Predicate != entities.PredicateDuty {
		t.Errorf("Predicate = %q, want %q", payload.Predicate, entities.PredicateDuty)
	}
}

func TestPolicy_Assign(t *testing.T) {
	t.Parallel()

	policy, err := new(entities.Policy).With("policy-1", "Test Policy", entities.PolicyTypeSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := policy.Assign("agent-1"); err != nil {
		t.Fatalf("Assign() error: %v", err)
	}

	events := policy.GetUncommittedEvents()
	lastEvent := events[len(events)-1]
	if lastEvent.EventType != entities.EventTypePolicyAssigned {
		t.Errorf("event type = %q, want %q", lastEvent.EventType, entities.EventTypePolicyAssigned)
	}

	payload, ok := lastEvent.Payload.(entities.PolicyAssigned)
	if !ok {
		t.Fatalf("expected PolicyAssigned payload, got %T", lastEvent.Payload)
	}
	if payload.Subject != "policy-1" {
		t.Errorf("Subject = %q, want %q", payload.Subject, "policy-1")
	}
	if payload.Predicate != entities.PredicateAssignee {
		t.Errorf("Predicate = %q, want %q", payload.Predicate, entities.PredicateAssignee)
	}
	if payload.Object != "agent-1" {
		t.Errorf("Object = %q, want %q", payload.Object, "agent-1")
	}
}

func TestPolicy_ValidationErrors(t *testing.T) {
	t.Parallel()

	policy, err := new(entities.Policy).With("policy-1", "Test Policy", entities.PolicyTypeSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// GrantPermission validation
	if err := policy.GrantPermission("", entities.ActionRead, "resource-1"); err == nil {
		t.Error("expected error for empty assignee")
	}
	if err := policy.GrantPermission("agent-1", "", "resource-1"); err == nil {
		t.Error("expected error for empty action")
	}
	if err := policy.GrantPermission("agent-1", entities.ActionRead, ""); err == nil {
		t.Error("expected error for empty target")
	}

	// Assign validation
	if err := policy.Assign(""); err == nil {
		t.Error("expected error for empty assignee ID")
	}
}

func TestPolicy_RevokePermission(t *testing.T) {
	t.Parallel()

	policy, err := new(entities.Policy).With("policy-1", "Test Policy", entities.PolicyTypeSet)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := policy.GrantPermission("agent-1", entities.ActionRead, "resource-1"); err != nil {
		t.Fatalf("GrantPermission() error: %v", err)
	}

	if err := policy.RevokePermission("agent-1", entities.ActionRead, "resource-1"); err != nil {
		t.Fatalf("RevokePermission() error: %v", err)
	}

	events := policy.GetUncommittedEvents()
	lastEvent := events[len(events)-1]
	if lastEvent.EventType != entities.EventTypePermissionRevoked {
		t.Errorf("event type = %q, want %q", lastEvent.EventType, entities.EventTypePermissionRevoked)
	}
}
