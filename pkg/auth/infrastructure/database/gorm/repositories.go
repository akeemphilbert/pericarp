package gorm

import (
	"context"
	"errors"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/models"
	"gorm.io/gorm"
)

// agentRepository implements repositories.AgentRepository using GORM.
type agentRepository struct {
	db *gorm.DB
}

// NewAgentRepository creates a new GORM-backed AgentRepository.
func NewAgentRepository(db *gorm.DB) repositories.AgentRepository {
	return &agentRepository{db: db}
}

func (r *agentRepository) Save(ctx context.Context, agent *entities.Agent) error {
	m := models.AgentModelFromEntity(agent)
	return r.db.WithContext(ctx).Save(m).Error
}

func (r *agentRepository) FindByID(ctx context.Context, id string) (*entities.Agent, error) {
	var m models.AgentModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return m.ToEntity()
}

func (r *agentRepository) FindAll(ctx context.Context, cursor string, limit int) (*repositories.PaginatedResponse[*entities.Agent], error) {
	if limit <= 0 {
		limit = 20
	}

	query := r.db.WithContext(ctx).Order("id ASC")
	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}

	var records []models.AgentModel
	if err := query.Limit(limit + 1).Find(&records).Error; err != nil {
		return nil, err
	}

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}

	result := &repositories.PaginatedResponse[*entities.Agent]{
		Data:    make([]*entities.Agent, 0, len(records)),
		Limit:   limit,
		HasMore: hasMore,
	}

	for i := range records {
		agent, err := records[i].ToEntity()
		if err != nil {
			return nil, err
		}
		result.Data = append(result.Data, agent)
	}

	if len(records) > 0 {
		result.Cursor = records[len(records)-1].ID
	}

	return result, nil
}

// credentialRepository implements repositories.CredentialRepository using GORM.
type credentialRepository struct {
	db *gorm.DB
}

// NewCredentialRepository creates a new GORM-backed CredentialRepository.
func NewCredentialRepository(db *gorm.DB) repositories.CredentialRepository {
	return &credentialRepository{db: db}
}

func (r *credentialRepository) Save(ctx context.Context, credential *entities.Credential) error {
	m := models.CredentialModelFromEntity(credential)
	return r.db.WithContext(ctx).Save(m).Error
}

func (r *credentialRepository) FindByID(ctx context.Context, id string) (*entities.Credential, error) {
	var m models.CredentialModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return m.ToEntity()
}

func (r *credentialRepository) FindByProvider(ctx context.Context, provider, providerUserID string) (*entities.Credential, error) {
	var m models.CredentialModel
	err := r.db.WithContext(ctx).Where("provider = ? AND provider_user_id = ?", provider, providerUserID).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return m.ToEntity()
}

func (r *credentialRepository) FindByEmail(ctx context.Context, email string) ([]*entities.Credential, error) {
	var records []models.CredentialModel
	if err := r.db.WithContext(ctx).Where("email = ?", email).Find(&records).Error; err != nil {
		return nil, err
	}

	result := make([]*entities.Credential, 0, len(records))
	for i := range records {
		cred, err := records[i].ToEntity()
		if err != nil {
			return nil, err
		}
		result = append(result, cred)
	}
	return result, nil
}

func (r *credentialRepository) FindByAgent(ctx context.Context, agentID string) ([]*entities.Credential, error) {
	var records []models.CredentialModel
	if err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).Find(&records).Error; err != nil {
		return nil, err
	}

	result := make([]*entities.Credential, 0, len(records))
	for i := range records {
		cred, err := records[i].ToEntity()
		if err != nil {
			return nil, err
		}
		result = append(result, cred)
	}
	return result, nil
}

func (r *credentialRepository) FindAll(ctx context.Context, cursor string, limit int) (*repositories.PaginatedResponse[*entities.Credential], error) {
	if limit <= 0 {
		limit = 20
	}

	query := r.db.WithContext(ctx).Order("id ASC")
	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}

	var records []models.CredentialModel
	if err := query.Limit(limit + 1).Find(&records).Error; err != nil {
		return nil, err
	}

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}

	result := &repositories.PaginatedResponse[*entities.Credential]{
		Data:    make([]*entities.Credential, 0, len(records)),
		Limit:   limit,
		HasMore: hasMore,
	}

	for i := range records {
		cred, err := records[i].ToEntity()
		if err != nil {
			return nil, err
		}
		result.Data = append(result.Data, cred)
	}

	if len(records) > 0 {
		result.Cursor = records[len(records)-1].ID
	}

	return result, nil
}

// authSessionRepository implements repositories.AuthSessionRepository using GORM.
type authSessionRepository struct {
	db *gorm.DB
}

// NewAuthSessionRepository creates a new GORM-backed AuthSessionRepository.
func NewAuthSessionRepository(db *gorm.DB) repositories.AuthSessionRepository {
	return &authSessionRepository{db: db}
}

func (r *authSessionRepository) Save(ctx context.Context, session *entities.AuthSession) error {
	m := models.AuthSessionModelFromEntity(session)
	return r.db.WithContext(ctx).Save(m).Error
}

func (r *authSessionRepository) FindByID(ctx context.Context, id string) (*entities.AuthSession, error) {
	var m models.AuthSessionModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return m.ToEntity()
}

func (r *authSessionRepository) FindByAgent(ctx context.Context, agentID string) ([]*entities.AuthSession, error) {
	var records []models.AuthSessionModel
	if err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).Find(&records).Error; err != nil {
		return nil, err
	}

	result := make([]*entities.AuthSession, 0, len(records))
	for i := range records {
		s, err := records[i].ToEntity()
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, nil
}

func (r *authSessionRepository) FindActive(ctx context.Context, agentID string) ([]*entities.AuthSession, error) {
	var records []models.AuthSessionModel
	err := r.db.WithContext(ctx).Where("agent_id = ? AND active = ? AND expires_at > ?", agentID, true, time.Now()).
		Find(&records).Error
	if err != nil {
		return nil, err
	}

	result := make([]*entities.AuthSession, 0, len(records))
	for i := range records {
		s, err := records[i].ToEntity()
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, nil
}

func (r *authSessionRepository) FindAll(ctx context.Context, cursor string, limit int) (*repositories.PaginatedResponse[*entities.AuthSession], error) {
	if limit <= 0 {
		limit = 20
	}

	query := r.db.WithContext(ctx).Order("id ASC")
	if cursor != "" {
		query = query.Where("id > ?", cursor)
	}

	var records []models.AuthSessionModel
	if err := query.Limit(limit + 1).Find(&records).Error; err != nil {
		return nil, err
	}

	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}

	result := &repositories.PaginatedResponse[*entities.AuthSession]{
		Data:    make([]*entities.AuthSession, 0, len(records)),
		Limit:   limit,
		HasMore: hasMore,
	}

	for i := range records {
		s, err := records[i].ToEntity()
		if err != nil {
			return nil, err
		}
		result.Data = append(result.Data, s)
	}

	if len(records) > 0 {
		result.Cursor = records[len(records)-1].ID
	}

	return result, nil
}

func (r *authSessionRepository) RevokeAllForAgent(ctx context.Context, agentID string) error {
	return r.db.WithContext(ctx).Model(&models.AuthSessionModel{}).
		Where("agent_id = ? AND active = ?", agentID, true).
		Update("active", false).Error
}
