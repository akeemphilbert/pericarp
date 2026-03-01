package entities

import (
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/domain"
)

// PolicyCreated represents the creation of a policy.
type PolicyCreated struct {
	Name       string    `json:"name"`
	PolicyType string    `json:"policy_type"`
	Timestamp  time.Time `json:"timestamp"`
}

// With creates a new PolicyCreated event.
func (e PolicyCreated) With(name, policyType string) PolicyCreated {
	return PolicyCreated{
		Name:       name,
		PolicyType: policyType,
		Timestamp:  time.Now(),
	}
}

// EventType returns the event type name.
func (e PolicyCreated) EventType() string {
	return EventTypePolicyCreated
}

// PolicyActivated represents the activation of a policy.
type PolicyActivated struct {
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new PolicyActivated event.
func (e PolicyActivated) With() PolicyActivated {
	return PolicyActivated{Timestamp: time.Now()}
}

// EventType returns the event type name.
func (e PolicyActivated) EventType() string {
	return EventTypePolicyActivated
}

// PolicyDeactivated represents the deactivation of a policy.
type PolicyDeactivated struct {
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new PolicyDeactivated event.
func (e PolicyDeactivated) With() PolicyDeactivated {
	return PolicyDeactivated{Timestamp: time.Now()}
}

// EventType returns the event type name.
func (e PolicyDeactivated) EventType() string {
	return EventTypePolicyDeactivated
}

// PermissionGranted represents granting a permission within a policy.
// Enriched triple: (Assignee, odrl:permission, Target) with action metadata.
type PermissionGranted struct {
	domain.BasicTripleEvent
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new PermissionGranted event.
func (e PermissionGranted) With(assignee, target, action string) PermissionGranted {
	return PermissionGranted{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   assignee,
			Predicate: PredicatePermission,
			Object:    target,
		},
		Action:    action,
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e PermissionGranted) EventType() string {
	return EventTypePermissionGranted
}

// PermissionRevoked represents revoking a previously granted permission.
// Enriched triple: (Assignee, odrl:permission, Target) with action metadata.
type PermissionRevoked struct {
	domain.BasicTripleEvent
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new PermissionRevoked event.
func (e PermissionRevoked) With(assignee, target, action string) PermissionRevoked {
	return PermissionRevoked{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   assignee,
			Predicate: PredicatePermission,
			Object:    target,
		},
		Action:    action,
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e PermissionRevoked) EventType() string {
	return EventTypePermissionRevoked
}

// ProhibitionSet represents setting a prohibition within a policy.
// Enriched triple: (Assignee, odrl:prohibition, Target) with action metadata.
type ProhibitionSet struct {
	domain.BasicTripleEvent
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new ProhibitionSet event.
func (e ProhibitionSet) With(assignee, target, action string) ProhibitionSet {
	return ProhibitionSet{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   assignee,
			Predicate: PredicateProhibition,
			Object:    target,
		},
		Action:    action,
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e ProhibitionSet) EventType() string {
	return EventTypeProhibitionSet
}

// ProhibitionRevoked represents revoking a previously set prohibition.
// Enriched triple: (Assignee, odrl:prohibition, Target) with action metadata.
type ProhibitionRevoked struct {
	domain.BasicTripleEvent
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new ProhibitionRevoked event.
func (e ProhibitionRevoked) With(assignee, target, action string) ProhibitionRevoked {
	return ProhibitionRevoked{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   assignee,
			Predicate: PredicateProhibition,
			Object:    target,
		},
		Action:    action,
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e ProhibitionRevoked) EventType() string {
	return EventTypeProhibitionRevoked
}

// DutyImposed represents imposing an obligation within a policy.
// Enriched triple: (Assignee, odrl:duty, Target) with action metadata.
type DutyImposed struct {
	domain.BasicTripleEvent
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new DutyImposed event.
func (e DutyImposed) With(assignee, target, action string) DutyImposed {
	return DutyImposed{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   assignee,
			Predicate: PredicateDuty,
			Object:    target,
		},
		Action:    action,
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e DutyImposed) EventType() string {
	return EventTypeDutyImposed
}

// DutyDischarged represents fulfilling an obligation within a policy.
// Enriched triple: (Assignee, odrl:duty, Target) with action metadata.
type DutyDischarged struct {
	domain.BasicTripleEvent
	Action    string    `json:"action"`
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new DutyDischarged event.
func (e DutyDischarged) With(assignee, target, action string) DutyDischarged {
	return DutyDischarged{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   assignee,
			Predicate: PredicateDuty,
			Object:    target,
		},
		Action:    action,
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e DutyDischarged) EventType() string {
	return EventTypeDutyDischarged
}

// PolicyAssigned represents assigning a policy to an agent or role.
// Triple: (Policy, odrl:assignee, Agent/Role)
type PolicyAssigned struct {
	domain.BasicTripleEvent
	Timestamp time.Time `json:"timestamp"`
}

// With creates a new PolicyAssigned event.
func (e PolicyAssigned) With(policyID, assigneeID string) PolicyAssigned {
	return PolicyAssigned{
		BasicTripleEvent: domain.BasicTripleEvent{
			Subject:   policyID,
			Predicate: PredicateAssignee,
			Object:    assigneeID,
		},
		Timestamp: time.Now(),
	}
}

// EventType returns the event type name.
func (e PolicyAssigned) EventType() string {
	return EventTypePolicyAssigned
}
