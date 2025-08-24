Feature: CQRS and Event Sourcing Behavior
  As a developer
  I want to validate CQRS and Event Sourcing patterns
  So that the system maintains proper separation and event-driven architecture

  Background:
    Given the system is running
    And the database is clean

  # Command-Query Separation
  Scenario: Commands should not return data
    When I create a user with email "test@example.com" and name "Test User"
    Then the command should complete successfully
    And the command should not return user data
    And I should query separately to get user details

  Scenario: Queries should not modify state
    Given a user exists with email "test@example.com" and name "Test User"
    When I query for the user by email "test@example.com"
    Then I should receive the user details
    And no events should be generated
    And the system state should remain unchanged

  # Event Sourcing Validation
  Scenario: All state changes generate events
    When I create a user with email "test@example.com" and name "Test User"
    Then a UserCreated event should be generated
    When I update the user's email to "updated@example.com"
    Then a UserEmailUpdated event should be generated
    When I deactivate the user
    Then a UserDeactivated event should be generated

  Scenario: Events contain all necessary data
    When I create a user with email "test@example.com" and name "Test User"
    Then the UserCreated event should contain:
      | field       | value              |
      | userId      | generated UUID     |
      | email       | test@example.com   |
      | name        | Test User          |
      | version     | 1                  |
      | timestamp   | current time       |

  Scenario: Event versioning is sequential
    Given a user exists with email "test@example.com" and name "Test User"
    When I update the user's email to "updated@example.com"
    And I update the user's name to "Updated User"
    And I deactivate the user
    Then the events should have sequential versions:
      | event_type         | version |
      | UserCreated        | 1       |
      | UserEmailUpdated   | 2       |
      | UserNameUpdated    | 3       |
      | UserDeactivated    | 4       |

  # Aggregate Reconstruction
  Scenario: Aggregate can be reconstructed from events
    Given a user exists with email "test@example.com" and name "Test User"
    When I update the user's email to "updated@example.com"
    And I update the user's name to "Updated User"
    And I deactivate the user
    When I reconstruct the user from events
    Then the reconstructed user should have:
      | field    | value              |
      | email    | updated@example.com |
      | name     | Updated User       |
      | active   | false              |
      | version  | 4                  |

  Scenario: Partial event replay
    Given a user exists with email "test@example.com" and name "Test User"
    And the user's email is updated to "updated@example.com"
    And the user's name is updated to "Updated User"
    And the user is deactivated
    When I reconstruct the user from events starting at version 2
    Then the reconstructed user should reflect changes from version 2 onwards
    And the user should have email "updated@example.com"
    And the user should have name "Updated User"
    And the user should be inactive

  # Event Store Behavior
  Scenario: Events are immutable once stored
    Given a user exists with email "test@example.com" and name "Test User"
    When the UserCreated event is stored
    Then the event should not be modifiable
    And any attempt to modify should fail
    And the original event should remain intact

  Scenario: Event ordering is preserved
    When I perform multiple operations in sequence:
      | operation                    | order |
      | Create user                  | 1     |
      | Update email                 | 2     |
      | Update name                  | 3     |
      | Deactivate user             | 4     |
    Then the events should be stored in the same order
    And replay should produce the same final state

  # Projection and Read Models
  Scenario: Read models are eventually consistent
    When I create a user with email "test@example.com" and name "Test User"
    Then the UserCreated event should be stored immediately
    And the read model should be updated eventually
    And queries should return the updated data

  Scenario: Read model updates are idempotent
    Given a user exists with email "test@example.com" and name "Test User"
    When the UserCreated event is processed multiple times
    Then the read model should remain consistent
    And no duplicate data should be created

  Scenario: Read model handles out-of-order events
    Given events are processed out of order
    When the projection processes the events
    Then the final read model state should be correct
    And the projection should handle version conflicts appropriately

  # Snapshotting (if implemented)
  Scenario: Snapshot creation for performance
    Given a user with many events exists
    When a snapshot is created at version 10
    Then the snapshot should contain the complete state at version 10
    And reconstruction should use the snapshot as a starting point
    And only events after version 10 should be replayed

  # Event Metadata
  Scenario: Events include correlation and causation metadata
    When I create a user with correlation ID "corr-123"
    Then the UserCreated event should include:
      | metadata_field   | value    |
      | correlation_id   | corr-123 |
      | causation_id     | cmd-456  |
      | user_id          | user-789 |
      | timestamp        | ISO-8601 |

  # Saga/Process Manager Behavior
  Scenario: Complex business process coordination
    When I create a user with email "test@example.com" and name "Test User"
    Then a UserCreated event should trigger welcome email process
    And the welcome email saga should be started
    And the saga should coordinate multiple steps

  # Event Upcasting (if implemented)
  Scenario: Event schema evolution
    Given events exist with old schema version
    When the system processes old events
    Then the events should be upcasted to current schema
    And the business logic should work with upcasted events

  # Performance and Scalability
  Scenario: Event store performance with large event streams
    Given a user with 1000 events exists
    When I reconstruct the user from events
    Then the reconstruction should complete within acceptable time
    And memory usage should remain reasonable

  Scenario: Concurrent event appending
    Given multiple commands are executed concurrently for the same aggregate
    When the events are appended to the event store
    Then the events should be appended atomically
    And version conflicts should be detected and handled

  # Consistency Guarantees
  Scenario: Strong consistency within aggregate boundary
    Given a user aggregate exists
    When multiple operations are performed on the same user
    Then all operations should see consistent state
    And the aggregate should maintain its invariants

  Scenario: Eventual consistency across aggregates
    Given multiple user aggregates exist
    When operations are performed across different users
    Then each aggregate should be internally consistent
    And cross-aggregate consistency should be eventual