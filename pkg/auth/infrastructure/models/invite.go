package models

import (
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

// InviteModel is the GORM model for the Invite aggregate.
type InviteModel struct {
	ID             string `gorm:"primaryKey"`
	AccountID      string `gorm:"not null;index"`
	Email          string `gorm:"not null;index"`
	RoleID         string `gorm:"not null"`
	InviterAgentID string `gorm:"not null"`
	InviteeAgentID string `gorm:"not null"`
	Status         string `gorm:"not null;index;default:pending"`
	ExpiresAt      time.Time
	AcceptedAt     time.Time
	CreatedAt      time.Time
}

func (InviteModel) TableName() string {
	return "invites"
}

// ToEntity converts the GORM model to an Invite domain entity.
func (m *InviteModel) ToEntity() (*entities.Invite, error) {
	e := &entities.Invite{}
	err := e.Restore(
		m.ID, m.AccountID, m.Email, m.RoleID,
		m.InviterAgentID, m.InviteeAgentID, m.Status,
		m.ExpiresAt, m.AcceptedAt, m.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// InviteModelFromEntity converts an Invite domain entity to a GORM model.
func InviteModelFromEntity(e *entities.Invite) *InviteModel {
	return &InviteModel{
		ID:             e.GetID(),
		AccountID:      e.AccountID(),
		Email:          e.Email(),
		RoleID:         e.RoleID(),
		InviterAgentID: e.InviterAgentID(),
		InviteeAgentID: e.InviteeAgentID(),
		Status:         e.Status(),
		ExpiresAt:      e.ExpiresAt(),
		AcceptedAt:     e.AcceptedAt(),
		CreatedAt:      e.CreatedAt(),
	}
}
