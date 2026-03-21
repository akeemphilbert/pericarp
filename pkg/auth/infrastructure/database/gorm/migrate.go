package gorm

import (
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/models"
	"gorm.io/gorm"
)

// AutoMigrate creates or updates all auth-related database tables.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&models.AgentModel{},
		&models.CredentialModel{},
		&models.AuthSessionModel{},
		&models.AccountModel{},
		&models.AccountMemberModel{},
		&models.InviteModel{},
	)
}
