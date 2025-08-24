Feature: SQLite Database Configuration
  As a developer
  I want to use SQLite as the database backend
  So that I can run the application without external database dependencies

  Background:
    Given the system is configured to use SQLite
    And the database is clean

  Scenario: SQLite database initialization
    When the system starts up
    Then the SQLite database should be created
    And the event store tables should be initialized
    And the read model tables should be initialized

  Scenario: SQLite file-based persistence
    Given the system is using file-based SQLite
    When I create a user with email "test@example.com" and name "Test User"
    And I restart the system
    Then the user should still exist in the database
    And the events should be persisted in the SQLite file

  Scenario: SQLite in-memory database for testing
    Given the system is using in-memory SQLite
    When I create a user with email "test@example.com" and name "Test User"
    Then the user should be created successfully
    And the data should exist only in memory

  Scenario: SQLite transaction handling
    Given the system is using SQLite
    When I create a user with email "test@example.com" and name "Test User"
    And the transaction is committed
    Then the user should be persisted
    And the events should be stored atomically

  Scenario: SQLite concurrent access
    Given the system is using SQLite with WAL mode
    When multiple operations access the database simultaneously
    Then all operations should complete successfully
    And data integrity should be maintained

  Scenario: SQLite database migration
    Given an existing SQLite database with old schema
    When the system starts with new schema requirements
    Then the database should be migrated automatically
    And existing data should be preserved

  Scenario: SQLite performance with large datasets
    Given the system is using SQLite
    When I create 1000 users
    Then all users should be created within acceptable time
    And query performance should remain acceptable

  Scenario: SQLite error handling
    Given the system is using SQLite
    When the database file becomes read-only
    Then write operations should fail gracefully
    And appropriate error messages should be returned

  Scenario: SQLite backup and restore
    Given the system is using file-based SQLite
    And I have created several users
    When I backup the database file
    And restore it to a new location
    Then all data should be available in the restored database