package models

import (
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

// CredentialModel is the GORM model for the Credential aggregate.
type CredentialModel struct {
	ID             string `gorm:"primaryKey"`
	AgentID        string `gorm:"not null;index"`
	Provider       string `gorm:"not null;uniqueIndex:idx_provider_user"`
	ProviderUserID string `gorm:"not null;uniqueIndex:idx_provider_user"`
	Email          string `gorm:"index:idx_email"`
	DisplayName    string
	Active         bool `gorm:"not null;default:true"`
	CreatedAt      time.Time
	LastUsedAt     time.Time
}

func (CredentialModel) TableName() string {
	return "credentials"
}

// ToEntity converts the GORM model to a Credential domain entity.
func (m *CredentialModel) ToEntity() (*entities.Credential, error) {
	e := &entities.Credential{}
	err := e.Restore(
		m.ID, m.AgentID, m.Provider, m.ProviderUserID,
		m.Email, m.DisplayName, m.Active, m.CreatedAt, m.LastUsedAt,
	)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// CredentialModelFromEntity converts a Credential domain entity to a GORM model.
func CredentialModelFromEntity(e *entities.Credential) *CredentialModel {
	return &CredentialModel{
		ID:             e.GetID(),
		AgentID:        e.AgentID(),
		Provider:       e.Provider(),
		ProviderUserID: e.ProviderUserID(),
		Email:          e.Email(),
		DisplayName:    e.DisplayName(),
		Active:         e.Active(),
		CreatedAt:      e.CreatedAt(),
		LastUsedAt:     e.LastUsedAt(),
	}
}
