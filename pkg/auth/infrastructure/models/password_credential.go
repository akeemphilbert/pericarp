package models

import (
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

// PasswordCredentialModel is the GORM model for the PasswordCredential
// aggregate. It is linked 1:1 to a Credential row of provider="password" by
// CredentialID; the bcrypt hash is stored only here, never on the
// CredentialModel.
type PasswordCredentialModel struct {
	ID             string `gorm:"primaryKey"`
	CredentialID   string `gorm:"not null;uniqueIndex:idx_password_credential_id"`
	Algorithm      string `gorm:"not null"`
	Hash           string `gorm:"not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastVerifiedAt time.Time
}

func (PasswordCredentialModel) TableName() string {
	return "password_credentials"
}

// ToEntity converts the GORM model to a PasswordCredential domain entity.
func (m *PasswordCredentialModel) ToEntity() (*entities.PasswordCredential, error) {
	e := &entities.PasswordCredential{}
	if err := e.Restore(m.ID, m.CredentialID, m.Algorithm, m.Hash, m.CreatedAt, m.UpdatedAt, m.LastVerifiedAt); err != nil {
		return nil, err
	}
	return e, nil
}

// PasswordCredentialModelFromEntity converts a PasswordCredential domain
// entity to a GORM model.
func PasswordCredentialModelFromEntity(e *entities.PasswordCredential) *PasswordCredentialModel {
	return &PasswordCredentialModel{
		ID:             e.GetID(),
		CredentialID:   e.CredentialID(),
		Algorithm:      e.Algorithm(),
		Hash:           e.Hash(),
		CreatedAt:      e.CreatedAt(),
		UpdatedAt:      e.UpdatedAt(),
		LastVerifiedAt: e.LastVerifiedAt(),
	}
}
