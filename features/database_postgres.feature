Feature: PostgreSQL Database Configuration
  As a developer
  I want to use PostgreSQL as the database backend
  So that I can run the application in production with a robust database

  Background:
    Given the system is configured to use PostgreSQL
    And the database is clean

  Scenario: PostgreSQL database connection
    When the system starts up
    Then it should connect to PostgreSQL successfully
    And the connection pool should be initialized
    And the database should be ready for operations

  Scenario: PostgreSQL schema initialization
    Given a fresh PostgreSQL database
    When the system starts up
    Then the event store tables should be created
    And the read model tables should be created
    And proper indexes should be created for performance

  Scenario: PostgreSQL transaction isolation
    Given the system is using PostgreSQL
    When I create a user with email "test@example.com" and name "Test User"
    And another transaction reads the data simultaneously
    Then the transaction isolation should be maintained
    And data consistency should be guaranteed

  Scenario: PostgreSQL connection pooling
    Given the system is configured with connection pooling
    When multiple concurrent operations are performed
    Then connections should be reused efficiently
    And the connection pool should not be exhausted

  Scenario: PostgreSQL performance with indexes
    Given the system is using PostgreSQL with proper indexes
    When I create 10000 users
    And I query users by email
    Then the query should use the email index
    And performance should be acceptable

  Scenario: PostgreSQL JSONB support for event metadata
    Given the system is using PostgreSQL
    When I create a user with complex metadata
    Then the metadata should be stored as JSONB
    And I should be able to query the metadata efficiently

  Scenario: PostgreSQL backup and point-in-time recovery
    Given the system is using PostgreSQL
    And I have created several users
    When a backup is taken
    And data is modified after the backup
    Then I should be able to restore to the backup point
    And the data should be consistent

  Scenario: PostgreSQL high availability
    Given the system is configured for high availability
    When the primary database becomes unavailable
    Then the system should failover to the replica
    And operations should continue with minimal disruption

  Scenario: PostgreSQL SSL connection
    Given the system is configured to use SSL
    When connecting to PostgreSQL
    Then the connection should be encrypted
    And SSL certificates should be validated

  Scenario: PostgreSQL error handling and retries
    Given the system is using PostgreSQL
    When a temporary connection error occurs
    Then the system should retry the operation
    And eventually succeed when the connection is restored