package gorm

import (
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/models"
	"gorm.io/gorm"
)

// AutoMigrate creates or updates the auth tables (agents, credentials, auth_sessions, accounts, account_members).
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.AgentModel{},
		&models.CredentialModel{},
		&models.AuthSessionModel{},
		&models.AccountModel{},
		&models.AccountMemberModel{},
	)
}
