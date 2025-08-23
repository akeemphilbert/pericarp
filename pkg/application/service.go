package application

import (
	"context"
	"fmt"

	"github.com/example/pericarp/pkg/domain"
)

// ApplicationService provides base functionality for application services
type ApplicationService struct {
	unitOfWork domain.UnitOfWork
	logger     domain.Logger
}

// NewApplicationService creates a new application service with UnitOfWork integration
func NewApplicationService(unitOfWork domain.UnitOfWork, logger domain.Logger) *ApplicationService {
	return &ApplicationService{
		unitOfWork: unitOfWork,
		logger:     logger,
	}
}

// ExecuteInTransaction executes a function within a unit of work transaction
func (s *ApplicationService) ExecuteInTransaction(ctx context.Context, fn func(ctx context.Context, uow domain.UnitOfWork) error) error {
	s.logger.Debug("Starting transaction")

	err := fn(ctx, s.unitOfWork)
	if err != nil {
		s.logger.Error("Transaction failed, rolling back", "error", err)
		if rollbackErr := s.unitOfWork.Rollback(); rollbackErr != nil {
			s.logger.Error("Failed to rollback transaction", "error", rollbackErr)
			return fmt.Errorf("transaction failed: %w, rollback failed: %v", err, rollbackErr)
		}
		return err
	}

	s.logger.Debug("Committing transaction")
	envelopes, err := s.unitOfWork.Commit(ctx)
	if err != nil {
		s.logger.Error("Failed to commit transaction", "error", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.logger.Debug("Transaction committed successfully", "events_count", len(envelopes))
	return nil
}

// GetUnitOfWork returns the unit of work instance
func (s *ApplicationService) GetUnitOfWork() domain.UnitOfWork {
	return s.unitOfWork
}

// GetLogger returns the logger instance
func (s *ApplicationService) GetLogger() domain.Logger {
	return s.logger
}
