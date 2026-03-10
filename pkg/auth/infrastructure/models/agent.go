package models

import (
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
)

// AgentModel is the GORM model for the Agent aggregate.
type AgentModel struct {
	ID        string `gorm:"primaryKey"`
	Name      string `gorm:"not null"`
	AgentType string `gorm:"not null;default:foaf:Person"`
	Active    bool   `gorm:"not null;default:true"`
	CreatedAt time.Time
}

func (AgentModel) TableName() string {
	return "agents"
}

// ToEntity converts the GORM model to an Agent domain entity.
func (m *AgentModel) ToEntity() (*entities.Agent, error) {
	e := &entities.Agent{}
	err := e.Restore(m.ID, m.Name, m.AgentType, m.Active, m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// AgentModelFromEntity converts an Agent domain entity to a GORM model.
func AgentModelFromEntity(e *entities.Agent) *AgentModel {
	return &AgentModel{
		ID:        e.GetID(),
		Name:      e.Name(),
		AgentType: e.AgentType(),
		Active:    e.Active(),
		CreatedAt: e.CreatedAt(),
	}
}
