package projection

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/akeemphilbert/pericarp/pkg/eventsourcing/infrastructure"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type testProduct struct {
	ID        string
	Type      string
	Name      string
	Amount    float64
	CreatedAt time.Time
}

type testProductConverter struct {
	resourceType string
}

func (c *testProductConverter) ResourceType() string {
	return c.resourceType
}

func (c *testProductConverter) ToModel(entity *testProduct) (*ResourceModel, error) {
	data := infrastructure.JSONB{
		"name":   entity.Name,
		"amount": entity.Amount,
	}
	return &ResourceModel{
		ID:        entity.ID,
		Data:      data,
		CreatedAt: entity.CreatedAt,
		UpdatedAt: entity.CreatedAt,
	}, nil
}

func (c *testProductConverter) FromModel(model *ResourceModel) (*testProduct, error) {
	raw, err := json.Marshal(model.Data)
	if err != nil {
		return nil, err
	}
	var fields struct {
		Name   string  `json:"name"`
		Amount float64 `json:"amount"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return &testProduct{
		ID:        model.ID,
		Type:      model.ResourceType,
		Name:      fields.Name,
		Amount:    fields.Amount,
		CreatedAt: model.CreatedAt,
	}, nil
}

func setupTestDB(t *testing.T, tableName string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	err = db.Table(tableName).AutoMigrate(&ResourceModel{})
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func setupTestRegistry() *Registry {
	reg := NewRegistry()
	reg.MustRegister("FinancialProduct", AsAbstract(), WithTable("financial_products"))
	reg.MustRegister("Loan", WithParent("FinancialProduct"))
	reg.MustRegister("DepositAccount", WithParent("FinancialProduct"))
	reg.MustRegister("Payment", WithParent("FinancialProduct"))
	return reg
}

func TestPolymorphicRepository_SaveAndFindByID(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t, "financial_products")
	reg := setupTestRegistry()
	conv := &testProductConverter{resourceType: "Loan"}

	repo, err := NewPolymorphicRepository[*testProduct](db, reg, conv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	product := &testProduct{
		ID:        "prod_001",
		Name:      "Home Loan",
		Amount:    250000,
		CreatedAt: now,
	}

	if err := repo.Save(ctx, product); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	found, err := repo.FindByID(ctx, "prod_001")
	if err != nil {
		t.Fatalf("find failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find product")
	}
	if found.Name != "Home Loan" {
		t.Errorf("expected name Home Loan, got %s", found.Name)
	}
	if found.Amount != 250000 {
		t.Errorf("expected amount 250000, got %f", found.Amount)
	}
	if found.Type != "Loan" {
		t.Errorf("expected type Loan, got %s", found.Type)
	}
}

func TestPolymorphicRepository_FindByID_NotFound(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t, "financial_products")
	reg := setupTestRegistry()
	conv := &testProductConverter{resourceType: "Loan"}

	repo, err := NewPolymorphicRepository[*testProduct](db, reg, conv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found, err := repo.FindByID(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != nil {
		t.Error("expected nil for nonexistent ID")
	}
}

func TestPolymorphicRepository_FindAll_FiltersByType(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t, "financial_products")
	reg := setupTestRegistry()
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	loanConv := &testProductConverter{resourceType: "Loan"}
	depositConv := &testProductConverter{resourceType: "DepositAccount"}

	loanRepo, _ := NewPolymorphicRepository[*testProduct](db, reg, loanConv)
	depositRepo, _ := NewPolymorphicRepository[*testProduct](db, reg, depositConv)

	_ = loanRepo.Save(ctx, &testProduct{ID: "a_loan_1", Name: "Home Loan", Amount: 100, CreatedAt: now})
	_ = loanRepo.Save(ctx, &testProduct{ID: "b_loan_2", Name: "Car Loan", Amount: 200, CreatedAt: now})
	_ = depositRepo.Save(ctx, &testProduct{ID: "c_dep_1", Name: "Savings", Amount: 500, CreatedAt: now})

	result, err := loanRepo.FindAll(ctx, "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 2 {
		t.Fatalf("expected 2 loans, got %d", len(result.Data))
	}
	for _, p := range result.Data {
		if p.Type != "Loan" {
			t.Errorf("expected type Loan, got %s", p.Type)
		}
	}

	result, err = depositRepo.FindAll(ctx, "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 1 {
		t.Fatalf("expected 1 deposit, got %d", len(result.Data))
	}
}

func TestPolymorphicRepository_FindAllByParentType(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t, "financial_products")
	reg := setupTestRegistry()
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	loanConv := &testProductConverter{resourceType: "Loan"}
	depositConv := &testProductConverter{resourceType: "DepositAccount"}
	paymentConv := &testProductConverter{resourceType: "Payment"}

	loanRepo, _ := NewPolymorphicRepository[*testProduct](db, reg, loanConv)
	depositRepo, _ := NewPolymorphicRepository[*testProduct](db, reg, depositConv)
	paymentRepo, _ := NewPolymorphicRepository[*testProduct](db, reg, paymentConv)

	_ = loanRepo.Save(ctx, &testProduct{ID: "a_loan", Name: "Home Loan", Amount: 100, CreatedAt: now})
	_ = depositRepo.Save(ctx, &testProduct{ID: "b_dep", Name: "Savings", Amount: 500, CreatedAt: now})
	_ = paymentRepo.Save(ctx, &testProduct{ID: "c_pay", Name: "Wire Transfer", Amount: 50, CreatedAt: now})

	result, err := loanRepo.FindAllByParentType(ctx, "FinancialProduct", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 3 {
		t.Fatalf("expected 3 products, got %d", len(result.Data))
	}

	typesSeen := map[string]bool{}
	for _, p := range result.Data {
		typesSeen[p.Type] = true
	}
	for _, expected := range []string{"Loan", "DepositAccount", "Payment"} {
		if !typesSeen[expected] {
			t.Errorf("expected to see type %s in results", expected)
		}
	}
}

func TestPolymorphicRepository_Pagination(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t, "financial_products")
	reg := setupTestRegistry()
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	conv := &testProductConverter{resourceType: "Loan"}
	repo, _ := NewPolymorphicRepository[*testProduct](db, reg, conv)

	for i := 0; i < 5; i++ {
		id := string(rune('a'+i)) + "_loan"
		_ = repo.Save(ctx, &testProduct{ID: id, Name: "Loan", Amount: float64(i * 100), CreatedAt: now})
	}

	page1, err := repo.FindAll(ctx, "", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page1.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(page1.Data))
	}
	if !page1.HasMore {
		t.Error("expected HasMore to be true")
	}
	if page1.Cursor == "" {
		t.Error("expected non-empty cursor")
	}

	page2, err := repo.FindAll(ctx, page1.Cursor, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page2.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(page2.Data))
	}
	if !page2.HasMore {
		t.Error("expected HasMore to be true")
	}

	page3, err := repo.FindAll(ctx, page2.Cursor, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page3.Data) != 1 {
		t.Fatalf("expected 1 item, got %d", len(page3.Data))
	}
	if page3.HasMore {
		t.Error("expected HasMore to be false")
	}
}

func TestPolymorphicRepository_Delete(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t, "financial_products")
	reg := setupTestRegistry()
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	conv := &testProductConverter{resourceType: "Loan"}
	repo, _ := NewPolymorphicRepository[*testProduct](db, reg, conv)

	_ = repo.Save(ctx, &testProduct{ID: "del_me", Name: "Temp", Amount: 0, CreatedAt: now})

	if err := repo.Delete(ctx, "del_me"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	found, err := repo.FindByID(ctx, "del_me")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found != nil {
		t.Error("expected nil after delete")
	}
}

func TestPolymorphicRepository_Update(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t, "financial_products")
	reg := setupTestRegistry()
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)

	conv := &testProductConverter{resourceType: "Loan"}
	repo, _ := NewPolymorphicRepository[*testProduct](db, reg, conv)

	_ = repo.Save(ctx, &testProduct{ID: "upd_1", Name: "V1", Amount: 100, CreatedAt: now})
	_ = repo.Save(ctx, &testProduct{ID: "upd_1", Name: "V2", Amount: 200, CreatedAt: now})

	found, err := repo.FindByID(ctx, "upd_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found.Name != "V2" {
		t.Errorf("expected name V2 after update, got %s", found.Name)
	}
	if found.Amount != 200 {
		t.Errorf("expected amount 200, got %f", found.Amount)
	}
}

func TestPolymorphicRepository_UnregisteredType(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t, "financial_products")
	reg := setupTestRegistry()
	conv := &testProductConverter{resourceType: "Unknown"}

	_, err := NewPolymorphicRepository[*testProduct](db, reg, conv)
	if err == nil {
		t.Fatal("expected error for unregistered type")
	}
}

func TestPolymorphicRepository_DefaultLimit(t *testing.T) {
	t.Parallel()
	db := setupTestDB(t, "financial_products")
	reg := setupTestRegistry()
	ctx := context.Background()

	conv := &testProductConverter{resourceType: "Loan"}
	repo, _ := NewPolymorphicRepository[*testProduct](db, reg, conv)

	result, err := repo.FindAll(ctx, "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Limit != 20 {
		t.Errorf("expected default limit 20, got %d", result.Limit)
	}
}

func TestPolymorphicRepository_FindAllByParentType_NoChildren(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.MustRegister("EmptyParent", AsAbstract(), WithTable("empty"))
	db := setupTestDB(t, "empty")

	conv := &testProductConverter{resourceType: "EmptyParent"}
	// EmptyParent is abstract, so it won't match as a converter type,
	// but we can still test FindAllByParentType on any repo using the same table.
	// We need a concrete type repo to call the method. Register a dummy type.
	reg.MustRegister("Dummy", WithParent("EmptyParent"))
	dummyConv := &testProductConverter{resourceType: "Dummy"}
	repo, err := NewPolymorphicRepository[*testProduct](db, reg, dummyConv)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = conv

	result, err := repo.FindAllByParentType(context.Background(), "EmptyParent", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Data) != 0 {
		t.Errorf("expected 0 results, got %d", len(result.Data))
	}
}
