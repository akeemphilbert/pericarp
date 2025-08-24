package domain

// DomainService defines the interface for domain services that contain
// business logic that doesn't naturally belong to any single aggregate.
//
// Domain services are used when:
//   - Business logic involves multiple aggregates
//   - The logic doesn't naturally fit within any single aggregate
//   - You need to coordinate between different domain concepts
//   - External domain knowledge is required for business decisions
//
// Domain services should:
//   - Contain only business logic, no infrastructure concerns
//   - Be stateless (no instance variables except dependencies)
//   - Use domain language and concepts
//   - Depend only on other domain objects and services
//
// Example domain service:
//
//	type TransferService struct {
//	    accountRepo AccountRepository
//	    feeCalculator FeeCalculator
//	}
//
//	func (s *TransferService) TransferMoney(
//	    fromAccountID, toAccountID string,
//	    amount Money,
//	) error {
//	    // Load both accounts
//	    fromAccount, err := s.accountRepo.Load(ctx, fromAccountID)
//	    if err != nil {
//	        return err
//	    }
//
//	    toAccount, err := s.accountRepo.Load(ctx, toAccountID)
//	    if err != nil {
//	        return err
//	    }
//
//	    // Calculate transfer fee (domain logic)
//	    fee := s.feeCalculator.CalculateFee(amount, fromAccount.Type(), toAccount.Type())
//
//	    // Perform transfer (coordinates multiple aggregates)
//	    if err := fromAccount.Withdraw(amount.Add(fee)); err != nil {
//	        return err
//	    }
//
//	    if err := toAccount.Deposit(amount); err != nil {
//	        // Compensate the withdrawal
//	        fromAccount.Deposit(amount.Add(fee))
//	        return err
//	    }
//
//	    return nil
//	}
//
// Common domain service patterns:
//   - Policy services (pricing, discounting, validation)
//   - Calculation services (tax, shipping, fees)
//   - Coordination services (transfers, workflows)
//   - Specification services (complex business rules)
type DomainService interface {
	// Domain services should define their own specific methods based on
	// the business logic they encapsulate. This is a marker interface
	// to identify domain services in the architecture.
	//
	// Concrete domain services should implement business-specific methods
	// that operate on domain objects and enforce business rules that
	// span multiple aggregates or require external domain knowledge.
}
