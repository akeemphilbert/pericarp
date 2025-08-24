Feature: Error Handling and Edge Cases
  As a developer
  I want the system to handle errors gracefully
  So that the application remains stable and provides meaningful feedback

  Background:
    Given the system is running
    And the database is clean

  # Domain Validation Errors
  Scenario: Invalid email format validation
    When I try to create a user with email "not-an-email" and name "Test User"
    Then the user creation should fail
    And the error should indicate "INVALID_EMAIL"
    And the error message should be descriptive

  Scenario: Email too long validation
    When I try to create a user with email "verylongemailaddressthatexceedsthemaximumlengthallowedbytheemailvalidation@verylongdomainnamethatalsoshouldberejected.com" and name "Test User"
    Then the user creation should fail
    And the error should indicate "EMAIL_TOO_LONG"

  Scenario: Name with special characters validation
    When I try to create a user with email "test@example.com" and name "Test<script>alert('xss')</script>User"
    Then the user creation should fail
    And the error should indicate "INVALID_NAME_CHARACTERS"

  # Infrastructure Errors
  Scenario: Database connection failure
    Given the database connection is lost
    When I try to create a user with email "test@example.com" and name "Test User"
    Then the user creation should fail
    And the error should indicate "DATABASE_CONNECTION_ERROR"
    And the system should attempt to reconnect

  Scenario: Event store write failure
    Given the event store is temporarily unavailable
    When I try to create a user with email "test@example.com" and name "Test User"
    Then the user creation should fail
    And the error should indicate "EVENT_STORE_ERROR"
    And no partial data should be persisted

  Scenario: Event dispatcher failure
    Given the event dispatcher is failing
    When I create a user with email "test@example.com" and name "Test User"
    Then the user should be created successfully
    But the event dispatch should fail
    And the error should be logged
    And the system should remain consistent

  # Concurrency Errors
  Scenario: Optimistic concurrency conflict
    Given a user exists with email "test@example.com" and name "Test User"
    When two concurrent updates are attempted on the same user
    Then one update should succeed
    And the other should fail with "CONCURRENCY_CONFLICT"
    And the successful update should be persisted

  Scenario: Deadlock detection and retry
    Given multiple users exist in the system
    When concurrent operations create a potential deadlock
    Then the system should detect the deadlock
    And retry the operations automatically
    And eventually complete all operations successfully

  # Resource Exhaustion
  Scenario: Memory exhaustion during bulk operations
    When I try to create 100000 users in a single operation
    Then the system should handle the load gracefully
    And either complete the operation or fail with "RESOURCE_EXHAUSTED"
    And the system should remain responsive

  Scenario: Connection pool exhaustion
    Given the connection pool has limited connections
    When more concurrent operations than available connections are attempted
    Then operations should queue appropriately
    And eventually complete when connections become available
    And no operations should be lost

  # Data Integrity Errors
  Scenario: Corrupted event data recovery
    Given events exist in the event store
    When an event becomes corrupted
    Then the system should detect the corruption
    And skip the corrupted event with appropriate logging
    And continue processing other events

  Scenario: Missing aggregate events
    Given a user aggregate exists
    When some events are missing from the event store
    Then the system should detect the gap
    And either reconstruct from available events or fail gracefully
    And log the data integrity issue

  # Network and Timeout Errors
  Scenario: Database query timeout
    Given the database is responding slowly
    When I perform a complex query that exceeds the timeout
    Then the query should be cancelled
    And a timeout error should be returned
    And the connection should be cleaned up properly

  Scenario: Network partition during event dispatch
    Given the system is running normally
    When a network partition occurs during event dispatch
    Then local operations should continue
    And events should be queued for later dispatch
    And the system should recover when the partition heals

  # Validation Edge Cases
  Scenario: Unicode characters in user data
    When I create a user with email "测试@example.com" and name "测试用户"
    Then the user should be created successfully
    And the Unicode characters should be preserved correctly

  Scenario: Very long valid email
    When I create a user with email "a.very.long.but.valid.email.address.that.is.within.limits@a-very-long-but-valid-domain-name.example.com" and name "Test User"
    Then the user should be created successfully

  Scenario: Boundary value testing for pagination
    Given 100 users exist in the system
    When I request page 1 with page size 2147483647
    Then the system should handle the large page size gracefully
    And return appropriate results or an error

  # Recovery Scenarios
  Scenario: System recovery after crash
    Given the system has processed several operations
    When the system crashes unexpectedly
    And the system is restarted
    Then the system should recover to a consistent state
    And all committed operations should be preserved

  Scenario: Partial transaction recovery
    Given a transaction is in progress
    When the system fails during the transaction
    And the system is restarted
    Then the partial transaction should be rolled back
    And the system should be in a consistent state

  # Security Edge Cases
  Scenario: SQL injection attempt in email field
    When I try to create a user with email "'; DROP TABLE users; --" and name "Test User"
    Then the user creation should fail with validation error
    And no SQL injection should occur
    And the database should remain intact

  Scenario: Very large payload attack
    When I try to create a user with a name containing 1MB of data
    Then the request should be rejected
    And the error should indicate "PAYLOAD_TOO_LARGE"
    And system resources should not be exhausted