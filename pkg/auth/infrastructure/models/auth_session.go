package models

import (
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

// AuthSessionModel is the GORM model for the AuthSession aggregate.
type AuthSessionModel struct {
	ID             string `gorm:"primaryKey"`
	AgentID        string `gorm:"not null;index"`
	AccountID      string
	CredentialID   string `gorm:"not null"`
	Active         bool   `gorm:"not null;default:true"`
	CreatedAt      time.Time
	ExpiresAt      time.Time
	LastAccessedAt time.Time
	IPAddress      string
	UserAgent      string
}

func (AuthSessionModel) TableName() string {
	return "auth_sessions"
}

// ToEntity converts the GORM model to an AuthSession domain entity.
func (m *AuthSessionModel) ToEntity() (*entities.AuthSession, error) {
	e := &entities.AuthSession{}
	err := e.Restore(
		m.ID, m.AgentID, m.AccountID, m.CredentialID,
		m.IPAddress, m.UserAgent, m.Active,
		m.CreatedAt, m.ExpiresAt, m.LastAccessedAt,
	)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// AuthSessionModelFromEntity converts an AuthSession domain entity to a GORM model.
func AuthSessionModelFromEntity(e *entities.AuthSession) *AuthSessionModel {
	return &AuthSessionModel{
		ID:             e.GetID(),
		AgentID:        e.AgentID(),
		AccountID:      e.AccountID(),
		CredentialID:   e.CredentialID(),
		Active:         e.Active(),
		CreatedAt:      e.CreatedAt(),
		ExpiresAt:      e.ExpiresAt(),
		LastAccessedAt: e.LastAccessedAt(),
		IPAddress:      e.IPAddress(),
		UserAgent:      e.UserAgent(),
	}
}
