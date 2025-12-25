package domain

import (
	"testing"
)

func TestEventTypeConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{
			name:     "EventTypeCreate",
			constant: EventTypeCreate,
			expected: "created",
		},
		{
			name:     "EventTypeUpdate",
			constant: EventTypeUpdate,
			expected: "updated",
		},
		{
			name:     "EventTypeDelete",
			constant: EventTypeDelete,
			expected: "deleted",
		},
		{
			name:     "EventTypeTriple",
			constant: EventTypeTriple,
			expected: "triple",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.constant != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, tt.constant)
			}
		})
	}
}

func TestEventTypeFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		entityType string
		action     string
		expected   string
	}{
		{
			name:       "user created",
			entityType: "user",
			action:     EventTypeCreate,
			expected:   "user.created",
		},
		{
			name:       "order updated",
			entityType: "order",
			action:     EventTypeUpdate,
			expected:   "order.updated",
		},
		{
			name:       "product deleted",
			entityType: "product",
			action:     EventTypeDelete,
			expected:   "product.deleted",
		},
		{
			name:       "relationship triple",
			entityType: "relationship",
			action:     EventTypeTriple,
			expected:   "relationship.triple",
		},
		{
			name:       "empty entity type",
			entityType: "",
			action:     EventTypeCreate,
			expected:   "created",
		},
		{
			name:       "empty action",
			entityType: "user",
			action:     "",
			expected:   "user",
		},
		{
			name:       "both empty",
			entityType: "",
			action:     "",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := EventTypeFor(tt.entityType, tt.action)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestIsStandardEventType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		eventType string
		expected  bool
	}{
		{
			name:      "created is standard",
			eventType: EventTypeCreate,
			expected:  true,
		},
		{
			name:      "updated is standard",
			eventType: EventTypeUpdate,
			expected:  true,
		},
		{
			name:      "deleted is standard",
			eventType: EventTypeDelete,
			expected:  true,
		},
		{
			name:      "triple is standard",
			eventType: EventTypeTriple,
			expected:  true,
		},
		{
			name:      "custom event type is not standard",
			eventType: "user.created",
			expected:  false,
		},
		{
			name:      "empty string is not standard",
			eventType: "",
			expected:  false,
		},
		{
			name:      "random string is not standard",
			eventType: "random.event",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsStandardEventType(tt.eventType)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
