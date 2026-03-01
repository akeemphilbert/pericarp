package entities_test

import (
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

func TestRole_With(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		id          string
		roleName    string
		description string
		wantErr     bool
	}{
		{
			name:        "creates role",
			id:          "role-admin",
			roleName:    "Administrator",
			description: "Full system access",
		},
		{
			name:        "creates role without description",
			id:          "role-viewer",
			roleName:    "Viewer",
			description: "",
		},
		{
			name:        "fails with empty ID",
			id:          "",
			roleName:    "Admin",
			description: "",
			wantErr:     true,
		},
		{
			name:        "fails with empty name",
			id:          "role-1",
			roleName:    "",
			description: "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			role, err := new(entities.Role).With(tt.id, tt.roleName, tt.description)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if role.GetID() != tt.id {
				t.Errorf("GetID() = %q, want %q", role.GetID(), tt.id)
			}
			if role.Name() != tt.roleName {
				t.Errorf("Name() = %q, want %q", role.Name(), tt.roleName)
			}
			if role.Description() != tt.description {
				t.Errorf("Description() = %q, want %q", role.Description(), tt.description)
			}
			if role.CreatedAt().IsZero() {
				t.Error("expected non-zero CreatedAt")
			}

			events := role.GetUncommittedEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 uncommitted event, got %d", len(events))
			}
			if events[0].EventType != entities.EventTypeRoleCreated {
				t.Errorf("event type = %q, want %q", events[0].EventType, entities.EventTypeRoleCreated)
			}
		})
	}
}
