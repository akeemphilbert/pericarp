package models

import (
	"fmt"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

// PasswordCredentialModel is the GORM model for the PasswordCredential
// aggregate. It is linked 1:1 to a Credential row of provider="password" by
// CredentialID; the bcrypt hash is stored only here, never on the
// CredentialModel.
//
// The "updated_at" column carries the domain meaning of "password rotated at"
// (see PasswordCredential.UpdatedAt). The struct field is intentionally named
// RotatedAt, not UpdatedAt, so GORM's auto-update timestamp magic does not
// touch it on Saves that only change LastVerifiedAt (i.e. successful logins).
type PasswordCredentialModel struct {
	ID             string `gorm:"primaryKey"`
	CredentialID   string `gorm:"not null;uniqueIndex:idx_password_credential_id"`
	Algorithm      string `gorm:"not null"`
	Hash           string `gorm:"not null"`
	CreatedAt      time.Time
	RotatedAt      time.Time `gorm:"column:updated_at"`
	LastVerifiedAt time.Time
}

func (PasswordCredentialModel) TableName() string {
	return "password_credentials"
}

// String redacts the Hash field so the model is safe to log via %v / %+v.
func (m PasswordCredentialModel) String() string {
	return fmt.Sprintf(
		"PasswordCredentialModel{ID:%s CredentialID:%s Algorithm:%s Hash:[REDACTED] CreatedAt:%s RotatedAt:%s LastVerifiedAt:%s}",
		m.ID, m.CredentialID, m.Algorithm, m.CreatedAt, m.RotatedAt, m.LastVerifiedAt,
	)
}

// GoString mirrors String so that %#v also redacts the hash.
func (m PasswordCredentialModel) GoString() string { return m.String() }

// ToEntity converts the GORM model to a PasswordCredential domain entity.
func (m *PasswordCredentialModel) ToEntity() (*entities.PasswordCredential, error) {
	e := &entities.PasswordCredential{}
	if err := e.Restore(m.ID, m.CredentialID, m.Algorithm, m.Hash, m.CreatedAt, m.RotatedAt, m.LastVerifiedAt); err != nil {
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
		RotatedAt:      e.UpdatedAt(),
		LastVerifiedAt: e.LastVerifiedAt(),
	}
}
