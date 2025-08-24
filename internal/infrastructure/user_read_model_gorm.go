package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/example/pericarp/internal/application"
	"gorm.io/gorm"
)

// UserReadModelGORM represents the GORM model for user read models
type UserReadModelGORM struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)"`
	Email     string    `gorm:"uniqueIndex;type:varchar(255);not null"`
	Name      string    `gorm:"type:varchar(255);not null"`
	IsActive  bool      `gorm:"not null;default:true"`
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// TableName specifies the table name for GORM
func (UserReadModelGORM) TableName() string {
	return "user_read_models"
}

// ToApplication converts GORM model to application model
func (u *UserReadModelGORM) ToApplication() *application.UserReadModel {
	id, _ := uuid.Parse(u.ID)
	return &application.UserReadModel{
		ID:        id,
		Email:     u.Email,
		Name:      u.Name,
		IsActive:  u.IsActive,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

// FromApplication converts application model to GORM model
func (u *UserReadModelGORM) FromApplication(user *application.UserReadModel) {
	u.ID = user.ID.String()
	u.Email = user.Email
	u.Name = user.Name
	u.IsActive = user.IsActive
	u.CreatedAt = user.CreatedAt
	u.UpdatedAt = user.UpdatedAt
}

// UserReadModelGORMRepository implements UserReadModelRepository using GORM
type UserReadModelGORMRepository struct {
	db *gorm.DB
}

// NewUserReadModelGORMRepository creates a new GORM-based user read model repository
func NewUserReadModelGORMRepository(db *gorm.DB) *UserReadModelGORMRepository {
	return &UserReadModelGORMRepository{db: db}
}

// GetByID retrieves a user read model by ID
func (r *UserReadModelGORMRepository) GetByID(ctx context.Context, id uuid.UUID) (*application.UserReadModel, error) {
	var user UserReadModelGORM
	
	result := r.db.WithContext(ctx).First(&user, "id = ?", id.String())
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by ID: %w", result.Error)
	}
	
	return user.ToApplication(), nil
}

// GetByEmail retrieves a user read model by email
func (r *UserReadModelGORMRepository) GetByEmail(ctx context.Context, email string) (*application.UserReadModel, error) {
	var user UserReadModelGORM
	
	result := r.db.WithContext(ctx).First(&user, "email = ?", email)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user by email: %w", result.Error)
	}
	
	return user.ToApplication(), nil
}

// List retrieves a paginated list of user read models with optional active filter
func (r *UserReadModelGORMRepository) List(ctx context.Context, page, pageSize int, active *bool) ([]application.UserReadModel, int, error) {
	var users []UserReadModelGORM
	var total int64
	
	// Build query with optional active filter
	query := r.db.WithContext(ctx).Model(&UserReadModelGORM{})
	if active != nil {
		query = query.Where("is_active = ?", *active)
	}
	
	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}
	
	// Calculate offset
	offset := (page - 1) * pageSize
	
	// Get paginated results
	result := query.
		Offset(offset).
		Limit(pageSize).
		Order("created_at DESC").
		Find(&users)
	
	if result.Error != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", result.Error)
	}
	
	// Convert to application models
	appUsers := make([]application.UserReadModel, len(users))
	for i, user := range users {
		appUsers[i] = *user.ToApplication()
	}
	
	return appUsers, int(total), nil
}

// Save saves or updates a user read model
func (r *UserReadModelGORMRepository) Save(ctx context.Context, user *application.UserReadModel) error {
	var gormUser UserReadModelGORM
	gormUser.FromApplication(user)
	
	// Use GORM's Save method which handles both insert and update
	result := r.db.WithContext(ctx).Save(&gormUser)
	if result.Error != nil {
		return fmt.Errorf("failed to save user: %w", result.Error)
	}
	
	return nil
}

// Delete removes a user read model
func (r *UserReadModelGORMRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).Delete(&UserReadModelGORM{}, "id = ?", id.String())
	if result.Error != nil {
		return fmt.Errorf("failed to delete user: %w", result.Error)
	}
	
	return nil
}

// Count returns the total number of users with optional active filter
func (r *UserReadModelGORMRepository) Count(ctx context.Context, active *bool) (int, error) {
	var count int64
	
	query := r.db.WithContext(ctx).Model(&UserReadModelGORM{})
	if active != nil {
		query = query.Where("is_active = ?", *active)
	}
	
	result := query.Count(&count)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to count users: %w", result.Error)
	}
	
	return int(count), nil
}

// Migrate creates the user read model table
func (r *UserReadModelGORMRepository) Migrate() error {
	return r.db.AutoMigrate(&UserReadModelGORM{})
}