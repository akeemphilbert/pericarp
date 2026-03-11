package models

import (
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

// AccountModel is the GORM model for the Account aggregate.
type AccountModel struct {
	ID          string `gorm:"primaryKey"`
	Name        string `gorm:"not null"`
	AccountType string `gorm:"not null;default:personal"`
	Active      bool   `gorm:"not null;default:true"`
	CreatedAt   time.Time
}

func (AccountModel) TableName() string {
	return "accounts"
}

// ToEntity converts the GORM model to an Account domain entity.
func (m *AccountModel) ToEntity() (*entities.Account, error) {
	e := &entities.Account{}
	err := e.Restore(m.ID, m.Name, m.AccountType, m.Active, m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// AccountModelFromEntity converts an Account domain entity to a GORM model.
func AccountModelFromEntity(e *entities.Account) *AccountModel {
	return &AccountModel{
		ID:          e.GetID(),
		Name:        e.Name(),
		AccountType: e.AccountType(),
		Active:      e.Active(),
		CreatedAt:   e.CreatedAt(),
	}
}

// AccountMemberModel is the GORM model for account membership (join table).
type AccountMemberModel struct {
	AccountID string    `gorm:"primaryKey"`
	AgentID   string    `gorm:"primaryKey;index:idx_account_member_agent"`
	RoleID    string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`
}

func (AccountMemberModel) TableName() string {
	return "account_members"
}

// AccountMemberModelFrom creates an AccountMemberModel from individual fields.
func AccountMemberModelFrom(accountID, agentID, roleID string) *AccountMemberModel {
	return &AccountMemberModel{
		AccountID: accountID,
		AgentID:   agentID,
		RoleID:    roleID,
		CreatedAt: time.Now(),
	}
}
