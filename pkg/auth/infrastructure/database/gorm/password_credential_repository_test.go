package gorm_test

import (
	"context"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	gorminfra "github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/database/gorm"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	if err := gorminfra.AutoMigrate(db); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestPasswordCredentialRepository_SaveAndFind(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := gorminfra.NewPasswordCredentialRepository(db)
	ctx := context.Background()

	pc, err := new(entities.PasswordCredential).With("pc-1", "cred-1", "bcrypt", "$2a$10$abc")
	if err != nil {
		t.Fatalf("create entity: %v", err)
	}
	if err := repo.Save(ctx, pc); err != nil {
		t.Fatalf("Save: %v", err)
	}

	found, err := repo.FindByID(ctx, "pc-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find by id, got nil")
	}
	if found.CredentialID() != "cred-1" {
		t.Errorf("CredentialID() = %q, want cred-1", found.CredentialID())
	}
	if found.Hash() != "$2a$10$abc" {
		t.Errorf("Hash() = %q, want $2a$10$abc", found.Hash())
	}

	byCred, err := repo.FindByCredentialID(ctx, "cred-1")
	if err != nil {
		t.Fatalf("FindByCredentialID: %v", err)
	}
	if byCred == nil {
		t.Fatal("expected to find by credential_id, got nil")
	}
	if byCred.GetID() != "pc-1" {
		t.Errorf("GetID() = %q, want pc-1", byCred.GetID())
	}
}

func TestPasswordCredentialRepository_NotFoundReturnsNilNil(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := gorminfra.NewPasswordCredentialRepository(db)
	ctx := context.Background()

	got, err := repo.FindByID(ctx, "missing")
	if err != nil {
		t.Fatalf("FindByID error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing id, got %+v", got)
	}

	got, err = repo.FindByCredentialID(ctx, "missing")
	if err != nil {
		t.Fatalf("FindByCredentialID error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing credential id, got %+v", got)
	}
}

func TestPasswordCredentialRepository_UpdateOverwrites(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := gorminfra.NewPasswordCredentialRepository(db)
	ctx := context.Background()

	pc, _ := new(entities.PasswordCredential).With("pc-1", "cred-1", "bcrypt", "$2a$10$abc")
	if err := repo.Save(ctx, pc); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := pc.Update("bcrypt", "$2a$12$new"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := repo.Save(ctx, pc); err != nil {
		t.Fatalf("Save (update): %v", err)
	}

	found, err := repo.FindByCredentialID(ctx, "cred-1")
	if err != nil {
		t.Fatalf("FindByCredentialID: %v", err)
	}
	if found.Hash() != "$2a$12$new" {
		t.Errorf("Hash() = %q, want $2a$12$new", found.Hash())
	}
}

func TestPasswordCredentialRepository_Delete(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	repo := gorminfra.NewPasswordCredentialRepository(db)
	ctx := context.Background()

	pc, _ := new(entities.PasswordCredential).With("pc-1", "cred-1", "bcrypt", "$2a$10$abc")
	if err := repo.Save(ctx, pc); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.Delete(ctx, "cred-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := repo.FindByCredentialID(ctx, "cred-1")
	if err != nil {
		t.Fatalf("FindByCredentialID after delete: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}

	// Idempotent: deleting again should not error.
	if err := repo.Delete(ctx, "cred-1"); err != nil {
		t.Fatalf("Delete (idempotent): %v", err)
	}
}
