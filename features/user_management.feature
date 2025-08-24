Feature: User Management
  As a system administrator
  I want to manage users in the system
  So that I can control access and maintain user data

  Background:
    Given the system is running
    And the database is clean

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

  Scenario: Updating user name
    Given a user exists with email "john@example.com" and name "John Doe"
    When I update the user's name to "John Smith"
    Then the name should be updated successfully
    And a UserNameUpdated event should be published
    And the read model should reflect the new name

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

  Scenario: Querying non-existent user fails
    When I query for a user with ID "00000000-0000-0000-0000-000000000000"
    Then the query should fail
    And the error should indicate "USER_NOT_FOUND"

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