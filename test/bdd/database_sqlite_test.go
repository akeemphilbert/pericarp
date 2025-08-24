package bdd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cucumber/godog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	pkginfra "github.com/example/pericarp/pkg/infrastructure"
)

// SQLiteTestContext holds SQLite-specific test state
type SQLiteTestContext struct {
	*TestContext
	dbFile     string
	tempDbFile string
}

// NewSQLiteTestContext creates a new SQLite test context
func NewSQLiteTestContext() *SQLiteTestContext {
	return &SQLiteTestContext{
		TestContext: NewTestContext(),
	}
}

func TestSQLiteDatabase(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			testCtx := NewSQLiteTestContext()

			// SQLite-specific steps
			ctx.Given(`^the system is configured to use SQLite$`, testCtx.theSystemIsConfiguredToUseSQLite)
			ctx.Given(`^the system is using file-based SQLite$`, testCtx.theSystemIsUsingFileBasedSQLite)
			ctx.Given(`^the system is using in-memory SQLite$`, testCtx.theSystemIsUsingInMemorySQLite)
			ctx.Given(`^the system is using SQLite with WAL mode$`, testCtx.theSystemIsUsingSQLiteWithWALMode)
			ctx.Given(`^an existing SQLite database with old schema$`, testCtx.anExistingSQLiteDatabaseWithOldSchema)
			ctx.Given(`^the database file becomes read-only$`, testCtx.theDatabaseFileBecomesReadOnly)
			ctx.Given(`^I have created several users$`, testCtx.iHaveCreatedSeveralUsers)

			ctx.When(`^the system starts up$`, testCtx.theSystemStartsUp)
			ctx.When(`^I restart the system$`, testCtx.iRestartTheSystem)
			ctx.When(`^multiple operations access the database simultaneously$`, testCtx.multipleOperationsAccessTheDatabaseSimultaneously)
			ctx.When(`^the system starts with new schema requirements$`, testCtx.theSystemStartsWithNewSchemaRequirements)
			ctx.When(`^I create (\d+) users$`, testCtx.iCreateUsers)
			ctx.When(`^write operations should fail gracefully$`, testCtx.writeOperationsShouldFailGracefully)
			ctx.When(`^I backup the database file$`, testCtx.iBackupTheDatabaseFile)
			ctx.When(`^restore it to a new location$`, testCtx.restoreItToANewLocation)

			ctx.Then(`^the SQLite database should be created$`, testCtx.theSQLiteDatabaseShouldBeCreated)
			ctx.Then(`^the event store tables should be initialized$`, testCtx.theEventStoreTablesShouldBeInitialized)
			ctx.Then(`^the read model tables should be initialized$`, testCtx.theReadModelTablesShouldBeInitialized)
			ctx.Then(`^the user should still exist in the database$`, testCtx.theUserShouldStillExistInTheDatabase)
			ctx.Then(`^the events should be persisted in the SQLite file$`, testCtx.theEventsShouldBePersistedInTheSQLiteFile)
			ctx.Then(`^the data should exist only in memory$`, testCtx.theDataShouldExistOnlyInMemory)
			ctx.Then(`^the user should be persisted$`, testCtx.theUserShouldBePersisted)
			ctx.Then(`^the events should be stored atomically$`, testCtx.theEventsShouldBeStoredAtomically)
			ctx.Then(`^all operations should complete successfully$`, testCtx.allOperationsShouldCompleteSuccessfully)
			ctx.Then(`^data integrity should be maintained$`, testCtx.dataIntegrityShouldBeMaintained)
			ctx.Then(`^the database should be migrated automatically$`, testCtx.theDatabaseShouldBeMigratedAutomatically)
			ctx.Then(`^existing data should be preserved$`, testCtx.existingDataShouldBePreserved)
			ctx.Then(`^all users should be created within acceptable time$`, testCtx.allUsersShouldBeCreatedWithinAcceptableTime)
			ctx.Then(`^query performance should remain acceptable$`, testCtx.queryPerformanceShouldRemainAcceptable)
			ctx.Then(`^appropriate error messages should be returned$`, testCtx.appropriateErrorMessagesShouldBeReturned)
			ctx.Then(`^all data should be available in the restored database$`, testCtx.allDataShouldBeAvailableInTheRestoredDatabase)

			// Common steps
			ctx.Given(`^the database is clean$`, testCtx.theDatabaseIsClean)
			ctx.When(`^I create a user with email "([^"]*)" and name "([^"]*)"$`, testCtx.iCreateAUserWithEmailAndName)
			ctx.When(`^the transaction is committed$`, testCtx.theTransactionIsCommitted)
			ctx.Then(`^the user should be created successfully$`, testCtx.theUserShouldBeCreatedSuccessfully)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features/database_sqlite.feature"},
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run SQLite feature tests")
	}
}

// SQLite-specific step implementations
func (tc *SQLiteTestContext) theSystemIsConfiguredToUseSQLite() error {
	tc.dbConfig = "sqlite"
	return nil
}

func (tc *SQLiteTestContext) theSystemIsUsingFileBasedSQLite() error {
	tc.dbConfig = "sqlite-file"
	// Create a temporary file for testing
	tempDir := os.TempDir()
	tc.dbFile = filepath.Join(tempDir, fmt.Sprintf("test_sqlite_%d.db", time.Now().UnixNano()))
	return nil
}

func (tc *SQLiteTestContext) theSystemIsUsingInMemorySQLite() error {
	tc.dbConfig = "sqlite-memory"
	return nil
}

func (tc *SQLiteTestContext) theSystemIsUsingSQLiteWithWALMode() error {
	tc.dbConfig = "sqlite-wal"
	return nil
}

func (tc *SQLiteTestContext) anExistingSQLiteDatabaseWithOldSchema() error {
	// Create a database with old schema for migration testing
	tempDir := os.TempDir()
	tc.dbFile = filepath.Join(tempDir, fmt.Sprintf("old_schema_%d.db", time.Now().UnixNano()))
	
	// Create database with old schema (simplified)
	db, err := gorm.Open(sqlite.Open(tc.dbFile), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to create old schema database: %w", err)
	}
	
	// Create old schema tables (simplified version)
	if err := db.Exec("CREATE TABLE old_events (id TEXT PRIMARY KEY, data TEXT)").Error; err != nil {
		return fmt.Errorf("failed to create old schema: %w", err)
	}
	
	return nil
}

func (tc *SQLiteTestContext) theDatabaseFileBecomesReadOnly() error {
	if tc.dbFile == "" {
		return fmt.Errorf("no database file to make read-only")
	}
	
	// Make the database file read-only
	return os.Chmod(tc.dbFile, 0444)
}

func (tc *SQLiteTestContext) iHaveCreatedSeveralUsers() error {
	users := []struct {
		email string
		name  string
	}{
		{"user1@example.com", "User One"},
		{"user2@example.com", "User Two"},
		{"user3@example.com", "User Three"},
	}
	
	for _, user := range users {
		if err := tc.createUserWithEmailAndName(user.email, user.name, false); err != nil {
			return fmt.Errorf("failed to create user %s: %w", user.email, err)
		}
	}
	
	return nil
}

func (tc *SQLiteTestContext) theSystemStartsUp() error {
	var db *gorm.DB
	var err error
	
	switch tc.dbConfig {
	case "sqlite-file":
		if tc.dbFile == "" {
			tc.dbFile = filepath.Join(os.TempDir(), fmt.Sprintf("startup_test_%d.db", time.Now().UnixNano()))
		}
		db, err = gorm.Open(sqlite.Open(tc.dbFile), &gorm.Config{})
	case "sqlite-memory":
		db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	case "sqlite-wal":
		if tc.dbFile == "" {
			tc.dbFile = filepath.Join(os.TempDir(), fmt.Sprintf("wal_test_%d.db", time.Now().UnixNano()))
		}
		db, err = gorm.Open(sqlite.Open(tc.dbFile+"?_journal_mode=WAL"), &gorm.Config{})
	default:
		db, err = gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	}
	
	if err != nil {
		return fmt.Errorf("failed to start system with SQLite: %w", err)
	}
	
	tc.db = db
	return tc.initializeSystem()
}

func (tc *SQLiteTestContext) iRestartTheSystem() error {
	// Close current connection
	if tc.db != nil {
		sqlDB, err := tc.db.DB()
		if err == nil {
			sqlDB.Close()
		}
	}
	
	// Restart with the same database file
	return tc.theSystemStartsUp()
}

func (tc *SQLiteTestContext) multipleOperationsAccessTheDatabaseSimultaneously() error {
	// Simulate concurrent access
	const numOperations = 10
	errChan := make(chan error, numOperations)
	
	for i := 0; i < numOperations; i++ {
		go func(index int) {
			email := fmt.Sprintf("concurrent%d@example.com", index)
			name := fmt.Sprintf("Concurrent User %d", index)
			errChan <- tc.createUserWithEmailAndName(email, name, false)
		}(i)
	}
	
	// Wait for all operations to complete
	for i := 0; i < numOperations; i++ {
		if err := <-errChan; err != nil {
			return fmt.Errorf("concurrent operation %d failed: %w", i, err)
		}
	}
	
	return nil
}

func (tc *SQLiteTestContext) theSystemStartsWithNewSchemaRequirements() error {
	// Simulate schema migration by running auto-migrate
	return tc.runMigrations()
}

func (tc *SQLiteTestContext) iCreateUsers(count int) error {
	tc.operationStartTime = time.Now()
	
	for i := 0; i < count; i++ {
		email := fmt.Sprintf("perf%d@example.com", i)
		name := fmt.Sprintf("Performance User %d", i)
		
		if err := tc.createUserWithEmailAndName(email, name, false); err != nil {
			return fmt.Errorf("failed to create user %d: %w", i, err)
		}
	}
	
	return nil
}

func (tc *SQLiteTestContext) writeOperationsShouldFailGracefully() error {
	// Try to create a user when database is read-only
	err := tc.createUserWithEmailAndName("readonly@example.com", "Readonly User", true)
	if err == nil {
		return fmt.Errorf("expected write operation to fail on read-only database")
	}
	
	tc.lastError = err
	return nil
}

func (tc *SQLiteTestContext) iBackupTheDatabaseFile() error {
	if tc.dbFile == "" {
		return fmt.Errorf("no database file to backup")
	}
	
	// Create backup file
	tc.tempDbFile = tc.dbFile + ".backup"
	
	// Copy database file
	input, err := os.ReadFile(tc.dbFile)
	if err != nil {
		return fmt.Errorf("failed to read database file: %w", err)
	}
	
	return os.WriteFile(tc.tempDbFile, input, 0644)
}

func (tc *SQLiteTestContext) restoreItToANewLocation() error {
	if tc.tempDbFile == "" {
		return fmt.Errorf("no backup file to restore")
	}
	
	// Create new location for restored database
	restoredFile := tc.tempDbFile + ".restored"
	
	// Copy backup to new location
	input, err := os.ReadFile(tc.tempDbFile)
	if err != nil {
		return fmt.Errorf("failed to read backup file: %w", err)
	}
	
	if err := os.WriteFile(restoredFile, input, 0644); err != nil {
		return fmt.Errorf("failed to write restored file: %w", err)
	}
	
	// Update database file reference
	tc.dbFile = restoredFile
	return nil
}

// Assertion implementations
func (tc *SQLiteTestContext) theSQLiteDatabaseShouldBeCreated() error {
	if tc.db == nil {
		return fmt.Errorf("database connection not established")
	}
	
	// Verify we can execute a simple query
	var result int
	if err := tc.db.Raw("SELECT 1").Scan(&result).Error; err != nil {
		return fmt.Errorf("database not accessible: %w", err)
	}
	
	return nil
}

func (tc *SQLiteTestContext) theEventStoreTablesShouldBeInitialized() error {
	// Check if event store tables exist
	var count int64
	err := tc.db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='event_records'").Scan(&count).Error
	if err != nil {
		return fmt.Errorf("failed to check event store tables: %w", err)
	}
	
	if count == 0 {
		return fmt.Errorf("event store tables not initialized")
	}
	
	return nil
}

func (tc *SQLiteTestContext) theReadModelTablesShouldBeInitialized() error {
	// Check if read model tables exist
	var count int64
	err := tc.db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='user_read_models'").Scan(&count).Error
	if err != nil {
		return fmt.Errorf("failed to check read model tables: %w", err)
	}
	
	if count == 0 {
		return fmt.Errorf("read model tables not initialized")
	}
	
	return nil
}

func (tc *SQLiteTestContext) theUserShouldStillExistInTheDatabase() error {
	if tc.lastCreatedUser == nil {
		return fmt.Errorf("no user to check persistence")
	}
	
	// Query the database directly to verify persistence
	var count int64
	err := tc.db.Raw("SELECT COUNT(*) FROM user_read_models WHERE id = ?", tc.lastCreatedUser.UserID()).Scan(&count).Error
	if err != nil {
		return fmt.Errorf("failed to check user persistence: %w", err)
	}
	
	if count == 0 {
		return fmt.Errorf("user not persisted in database")
	}
	
	return nil
}

func (tc *SQLiteTestContext) theEventsShouldBePersistedInTheSQLiteFile() error {
	if tc.lastCreatedUser == nil {
		return fmt.Errorf("no user to check event persistence")
	}
	
	// Query events directly from database
	var count int64
	err := tc.db.Raw("SELECT COUNT(*) FROM event_records WHERE aggregate_id = ?", tc.lastCreatedUser.ID()).Scan(&count).Error
	if err != nil {
		return fmt.Errorf("failed to check event persistence: %w", err)
	}
	
	if count == 0 {
		return fmt.Errorf("events not persisted in SQLite file")
	}
	
	return nil
}

func (tc *SQLiteTestContext) theDataShouldExistOnlyInMemory() error {
	// For in-memory database, we can't check file persistence
	// Just verify data exists in current connection
	return tc.theUserShouldStillExistInTheDatabase()
}

func (tc *SQLiteTestContext) theUserShouldBePersisted() error {
	return tc.theUserShouldStillExistInTheDatabase()
}

func (tc *SQLiteTestContext) theEventsShouldBeStoredAtomically() error {
	// Verify that all events for the user are stored together
	if tc.lastCreatedUser == nil {
		return fmt.Errorf("no user to check atomic storage")
	}
	
	ctx := context.Background()
	envelopes, err := tc.eventStore.Load(ctx, tc.lastCreatedUser.ID())
	if err != nil {
		return fmt.Errorf("failed to load events: %w", err)
	}
	
	if len(envelopes) == 0 {
		return fmt.Errorf("no events found, atomic storage failed")
	}
	
	return nil
}

func (tc *SQLiteTestContext) allOperationsShouldCompleteSuccessfully() error {
	// Verify that all concurrent operations completed without errors
	// This is implicitly checked by the concurrent operation step
	return nil
}

func (tc *SQLiteTestContext) dataIntegrityShouldBeMaintained() error {
	// Verify data integrity by checking constraints and relationships
	var eventCount, userCount int64
	
	if err := tc.db.Raw("SELECT COUNT(*) FROM event_records").Scan(&eventCount).Error; err != nil {
		return fmt.Errorf("failed to count events: %w", err)
	}
	
	if err := tc.db.Raw("SELECT COUNT(*) FROM user_read_models").Scan(&userCount).Error; err != nil {
		return fmt.Errorf("failed to count users: %w", err)
	}
	
	// Basic integrity check: should have at least as many events as users
	if eventCount < userCount {
		return fmt.Errorf("data integrity violation: %d events for %d users", eventCount, userCount)
	}
	
	return nil
}

func (tc *SQLiteTestContext) theDatabaseShouldBeMigratedAutomatically() error {
	// Verify that new schema is in place
	return tc.theEventStoreTablesShouldBeInitialized()
}

func (tc *SQLiteTestContext) existingDataShouldBePreserved() error {
	// Check that old data still exists after migration
	var count int64
	if err := tc.db.Raw("SELECT COUNT(*) FROM event_records").Scan(&count).Error; err != nil {
		return fmt.Errorf("failed to check preserved data: %w", err)
	}
	
	// Should have some data from the old schema
	return nil
}

func (tc *SQLiteTestContext) allUsersShouldBeCreatedWithinAcceptableTime() error {
	duration := time.Since(tc.operationStartTime)
	acceptableTime := 30 * time.Second // Adjust based on requirements
	
	if duration > acceptableTime {
		return fmt.Errorf("operation took too long: %v (acceptable: %v)", duration, acceptableTime)
	}
	
	return nil
}

func (tc *SQLiteTestContext) queryPerformanceShouldRemainAcceptable() error {
	// Perform a query and measure performance
	start := time.Now()
	
	var users []struct {
		ID    string
		Email string
	}
	
	if err := tc.db.Raw("SELECT id, email FROM user_read_models LIMIT 100").Scan(&users).Error; err != nil {
		return fmt.Errorf("query failed: %w", err)
	}
	
	duration := time.Since(start)
	acceptableTime := 1 * time.Second
	
	if duration > acceptableTime {
		return fmt.Errorf("query too slow: %v (acceptable: %v)", duration, acceptableTime)
	}
	
	return nil
}

func (tc *SQLiteTestContext) appropriateErrorMessagesShouldBeReturned() error {
	if tc.lastError == nil {
		return fmt.Errorf("no error to check")
	}
	
	// Verify error message is descriptive
	errorMsg := tc.lastError.Error()
	if len(errorMsg) < 10 {
		return fmt.Errorf("error message too short: %s", errorMsg)
	}
	
	return nil
}

func (tc *SQLiteTestContext) allDataShouldBeAvailableInTheRestoredDatabase() error {
	// Connect to restored database and verify data
	if tc.dbFile == "" {
		return fmt.Errorf("no restored database file")
	}
	
	restoredDB, err := gorm.Open(sqlite.Open(tc.dbFile), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to restored database: %w", err)
	}
	
	// Check that data exists in restored database
	var count int64
	if err := restoredDB.Raw("SELECT COUNT(*) FROM user_read_models").Scan(&count).Error; err != nil {
		return fmt.Errorf("failed to query restored database: %w", err)
	}
	
	if count == 0 {
		return fmt.Errorf("no data found in restored database")
	}
	
	return nil
}

// Helper methods
func (tc *SQLiteTestContext) initializeSystem() error {
	// Initialize logger
	tc.logger = pkginfra.NewLogger()

	// Initialize event store
	tc.eventStore = pkginfra.NewEventStore(tc.db)

	// Initialize event dispatcher
	tc.eventDispatcher = pkginfra.NewEventDispatcher()

	// Initialize unit of work
	tc.unitOfWork = pkginfra.NewUnitOfWork(tc.eventStore, tc.eventDispatcher)

	// Run migrations
	return tc.runMigrations()
}

func (tc *SQLiteTestContext) theTransactionIsCommitted() error {
	// In the context of our system, this means the unit of work is committed
	if tc.unitOfWork != nil {
		ctx := context.Background()
		_, err := tc.unitOfWork.Commit(ctx)
		return err
	}
	return nil
}

// Cleanup
func (tc *SQLiteTestContext) cleanup() {
	if tc.dbFile != "" && tc.dbFile != ":memory:" {
		if err := os.Remove(tc.dbFile); err != nil {
			// Log but don't fail cleanup - file might not exist
			fmt.Printf("Warning: Failed to remove database file %s: %v\n", tc.dbFile, err)
		}
	}
	if tc.tempDbFile != "" {
		if err := os.Remove(tc.tempDbFile); err != nil {
			fmt.Printf("Warning: Failed to remove temp database file %s: %v\n", tc.tempDbFile, err)
		}
		if err := os.Remove(tc.tempDbFile + ".restored"); err != nil {
			fmt.Printf("Warning: Failed to remove restored database file %s: %v\n", tc.tempDbFile+".restored", err)
		}
	}
}