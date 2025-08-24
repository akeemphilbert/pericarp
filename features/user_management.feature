Feature: User Management
  As a system administrator
  I want to manage users in the system
  So that I can control access and maintain user data

  Background:
    Given the system is running
    And the database is clean

  # Basic User Creation Scenarios
  Scenario: Creating a new user
    When I create a user with email "john@example.com" and name "John Doe"
    Then the user should be created successfully
    And a UserCreated event should be published
    And the user should appear in the read model

  Scenario: Creating a user with duplicate email fails
    Given a user exists with email "john@example.com" and name "John Doe"
    When I try to create a user with email "john@example.com" and name "Jane Smith"
    Then the user creation should fail
    And the error should indicate "EMAIL_ALREADY_EXISTS"

  # Input Validation Scenarios
  Scenario: Creating a user with invalid email fails
    When I try to create a user with email "invalid-email" and name "John Doe"
    Then the user creation should fail
    And the error should indicate "INVALID_EMAIL"

  Scenario: Creating a user with empty name fails
    When I try to create a user with email "john@example.com" and name ""
    Then the user creation should fail
    And the error should indicate "INVALID_NAME"

  Scenario: Creating a user with empty email fails
    When I try to create a user with email "" and name "John Doe"
    Then the user creation should fail
    And the error should indicate "INVALID_EMAIL"

  Scenario: Creating a user with very long name fails
    When I try to create a user with email "john@example.com" and name "This is a very long name that exceeds the maximum allowed length for user names in the system and should be rejected by validation"
    Then the user creation should fail
    And the error should indicate "NAME_TOO_LONG"

  # User Update Scenarios
  Scenario: Updating user email
    Given a user exists with email "john@example.com" and name "John Doe"
    When I update the user's email to "john.doe@example.com"
    Then the email should be updated successfully
    And a UserEmailUpdated event should be published
    And the read model should reflect the new email

  Scenario: Updating user email to existing email fails
    Given a user exists with email "john@example.com" and name "John Doe"
    And a user exists with email "jane@example.com" and name "Jane Smith"
    When I try to update the first user's email to "jane@example.com"
    Then the email update should fail
    And the error should indicate "EMAIL_ALREADY_EXISTS"

  Scenario: Updating user email to invalid format fails
    Given a user exists with email "john@example.com" and name "John Doe"
    When I try to update the user's email to "invalid-email"
    Then the email update should fail
    And the error should indicate "INVALID_EMAIL"

  Scenario: Updating user name
    Given a user exists with email "john@example.com" and name "John Doe"
    When I update the user's name to "John Smith"
    Then the name should be updated successfully
    And a UserNameUpdated event should be published
    And the read model should reflect the new name

  Scenario: Updating user name to empty value fails
    Given a user exists with email "john@example.com" and name "John Doe"
    When I try to update the user's name to ""
    Then the name update should fail
    And the error should indicate "INVALID_NAME"

  # User Activation/Deactivation Scenarios
  Scenario: Deactivating a user
    Given a user exists with email "john@example.com" and name "John Doe"
    When I deactivate the user
    Then the user should be deactivated successfully
    And a UserDeactivated event should be published
    And the read model should show the user as inactive

  Scenario: Activating a deactivated user
    Given a user exists with email "john@example.com" and name "John Doe"
    And the user is deactivated
    When I activate the user
    Then the user should be activated successfully
    And a UserActivated event should be published
    And the read model should show the user as active

  Scenario: Deactivating an already inactive user fails
    Given a user exists with email "john@example.com" and name "John Doe"
    And the user is deactivated
    When I try to deactivate the user
    Then the deactivation should fail
    And the error should indicate "USER_ALREADY_INACTIVE"

  Scenario: Activating an already active user fails
    Given a user exists with email "john@example.com" and name "John Doe"
    When I try to activate the user
    Then the activation should fail
    And the error should indicate "USER_ALREADY_ACTIVE"

  # Query Scenarios
  Scenario: Querying user by ID
    Given a user exists with email "john@example.com" and name "John Doe"
    When I query for the user by ID
    Then I should receive the user details
    And the details should match the created user

  Scenario: Querying user by email
    Given a user exists with email "john@example.com" and name "John Doe"
    When I query for the user by email "john@example.com"
    Then I should receive the user details
    And the details should match the created user

  Scenario: Querying non-existent user by ID fails
    When I query for a user with ID "00000000-0000-0000-0000-000000000000"
    Then the query should fail
    And the error should indicate "USER_NOT_FOUND"

  Scenario: Querying non-existent user by email fails
    When I query for the user by email "nonexistent@example.com"
    Then the query should fail
    And the error should indicate "USER_NOT_FOUND"

  # Pagination and Listing Scenarios
  Scenario: Listing users with pagination
    Given the following users exist:
      | email              | name        | active |
      | john@example.com   | John Doe    | true   |
      | jane@example.com   | Jane Smith  | true   |
      | bob@example.com    | Bob Johnson | false  |
    When I list users with page 1 and page size 2
    Then I should receive 2 users
    And the total count should be 3
    And the total pages should be 2

  Scenario: Listing only active users
    Given the following users exist:
      | email              | name        | active |
      | john@example.com   | John Doe    | true   |
      | jane@example.com   | Jane Smith  | true   |
      | bob@example.com    | Bob Johnson | false  |
    When I list active users with page 1 and page size 10
    Then I should receive 2 users
    And all users should be active

  Scenario: Listing users with empty result
    When I list users with page 1 and page size 10
    Then I should receive 0 users
    And the total count should be 0
    And the total pages should be 0

  Scenario: Listing users with invalid page number
    Given a user exists with email "john@example.com" and name "John Doe"
    When I try to list users with page 0 and page size 10
    Then the query should fail
    And the error should indicate "INVALID_PAGE"

  Scenario: Listing users with invalid page size
    Given a user exists with email "john@example.com" and name "John Doe"
    When I try to list users with page 1 and page size 0
    Then the query should fail
    And the error should indicate "INVALID_PAGE_SIZE"

  # Event Sourcing and CQRS Scenarios
  Scenario: Event sourcing - user state reconstruction
    Given a user exists with email "john@example.com" and name "John Doe"
    And the user's email is updated to "john.doe@example.com"
    And the user's name is updated to "John Smith"
    And the user is deactivated
    When I reconstruct the user from events
    Then the user should have email "john.doe@example.com"
    And the user should have name "John Smith"
    And the user should be inactive
    And the user version should be 4

  Scenario: Event ordering verification
    Given a user exists with email "john@example.com" and name "John Doe"
    When I update the user's email to "john.doe@example.com"
    And I update the user's name to "John Smith"
    And I deactivate the user
    Then the events should be in correct order
    And each event should have incremental version numbers

  Scenario: Concurrent modification detection
    Given a user exists with email "john@example.com" and name "John Doe"
    When I try to update the user with an outdated version
    Then the update should fail
    And the error should indicate "CONCURRENCY_CONFLICT"

  # Performance and Load Scenarios
  Scenario: Creating multiple users in sequence
    When I create 10 users with sequential emails
    Then all users should be created successfully
    And all UserCreated events should be published
    And all users should appear in the read model

  Scenario: Bulk user operations
    Given the following users exist:
      | email                | name          | active |
      | user1@example.com    | User One      | true   |
      | user2@example.com    | User Two      | true   |
      | user3@example.com    | User Three    | true   |
      | user4@example.com    | User Four     | true   |
      | user5@example.com    | User Five     | true   |
    When I list users with page 1 and page size 3
    Then I should receive 3 users
    And the pagination should work correctly

  # Error Recovery Scenarios
  Scenario: System recovery after event store failure
    Given a user exists with email "john@example.com" and name "John Doe"
    When the event store becomes temporarily unavailable
    And I try to update the user's email to "john.doe@example.com"
    Then the update should fail gracefully
    And the system should remain in a consistent state

  Scenario: Read model consistency after projection failure
    Given a user exists with email "john@example.com" and name "John Doe"
    When the projection fails temporarily
    And I update the user's email to "john.doe@example.com"
    Then the event should be stored successfully
    And the read model should eventually become consistent