package domain

// Standard event type constants for common domain operations.
const (
	// EventTypeCreate represents an entity creation event.
	EventTypeCreate = "created"

	// EventTypeUpdate represents an entity update event.
	EventTypeUpdate = "updated"

	// EventTypeDelete represents an entity deletion event.
	EventTypeDelete = "deleted"

	// EventTypeTriple represents an RDF-style relationship event (subject-predicate-object).
	EventTypeTriple = "triple"
)

// EventTypeFor constructs an event type string from an entity type and action.
// For example, EventTypeFor("user", EventTypeCreate) returns "user.created".
func EventTypeFor(entityType, action string) string {
	if entityType == "" {
		return action
	}
	if action == "" {
		return entityType
	}
	return entityType + "." + action
}

// IsStandardEventType checks if the given event type is one of the standard types.
func IsStandardEventType(eventType string) bool {
	return eventType == EventTypeCreate ||
		eventType == EventTypeUpdate ||
		eventType == EventTypeDelete ||
		eventType == EventTypeTriple
}
