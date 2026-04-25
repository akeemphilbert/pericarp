package gorm

import (
	"context"
	"errors"

	"github.com/akeemphilbert/pericarp/pkg/auth/domain/entities"
	"github.com/akeemphilbert/pericarp/pkg/auth/domain/repositories"
	"github.com/akeemphilbert/pericarp/pkg/auth/infrastructure/models"
	"gorm.io/gorm"
)

// passwordCredentialRepository implements repositories.PasswordCredentialRepository
// using GORM.
type passwordCredentialRepository struct {
	db *gorm.DB
}

// NewPasswordCredentialRepository creates a new GORM-backed
// PasswordCredentialRepository.
func NewPasswordCredentialRepository(db *gorm.DB) repositories.PasswordCredentialRepository {
	return &passwordCredentialRepository{db: db}
}

func (r *passwordCredentialRepository) Save(ctx context.Context, pc *entities.PasswordCredential) error {
	m := models.PasswordCredentialModelFromEntity(pc)
	return r.db.WithContext(ctx).Save(m).Error
}

func (r *passwordCredentialRepository) FindByID(ctx context.Context, id string) (*entities.PasswordCredential, error) {
	var m models.PasswordCredentialModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return m.ToEntity()
}

func (r *passwordCredentialRepository) FindByCredentialID(ctx context.Context, credentialID string) (*entities.PasswordCredential, error) {
	var m models.PasswordCredentialModel
	if err := r.db.WithContext(ctx).Where("credential_id = ?", credentialID).First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return m.ToEntity()
}

func (r *passwordCredentialRepository) Delete(ctx context.Context, credentialID string) error {
	return r.db.WithContext(ctx).Where("credential_id = ?", credentialID).Delete(&models.PasswordCredentialModel{}).Error
}
