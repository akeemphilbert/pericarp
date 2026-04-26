package projection

import (
	"errors"
	"testing"
)

func TestRegistry_RegisterAbstract(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	err := reg.Register("FinancialProduct", AsAbstract(), WithTable("financial_products"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !reg.IsAbstract("FinancialProduct") {
		t.Error("expected FinancialProduct to be abstract")
	}

	if !reg.IsRegistered("FinancialProduct") {
		t.Error("expected FinancialProduct to be registered")
	}
}

func TestRegistry_RegisterConcrete(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.MustRegister("FinancialProduct", AsAbstract(), WithTable("financial_products"))
	reg.MustRegister("Loan", WithParent("FinancialProduct"))

	if reg.IsAbstract("Loan") {
		t.Error("expected Loan to not be abstract")
	}

	table, err := reg.GetTable("Loan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if table != "financial_products" {
		t.Errorf("expected table financial_products, got %s", table)
	}
}

func TestRegistry_GetConcreteTypes(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.MustRegister("FinancialProduct", AsAbstract(), WithTable("financial_products"))
	reg.MustRegister("Loan", WithParent("FinancialProduct"))
	reg.MustRegister("DepositAccount", WithParent("FinancialProduct"))
	reg.MustRegister("Payment", WithParent("FinancialProduct"))

	types, err := reg.GetConcreteTypes("FinancialProduct")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"DepositAccount", "Loan", "Payment"}
	if len(types) != len(expected) {
		t.Fatalf("expected %d types, got %d: %v", len(expected), len(types), types)
	}
	for i, name := range expected {
		if types[i] != name {
			t.Errorf("expected type[%d] = %s, got %s", i, name, types[i])
		}
	}
}

func TestRegistry_GetConcreteTypes_Concrete(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.MustRegister("FinancialProduct", AsAbstract(), WithTable("financial_products"))
	reg.MustRegister("Loan", WithParent("FinancialProduct"))

	types, err := reg.GetConcreteTypes("Loan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(types) != 1 || types[0] != "Loan" {
		t.Errorf("expected [Loan], got %v", types)
	}
}

func TestRegistry_GetConcreteTypes_Unregistered(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	_, err := reg.GetConcreteTypes("Unknown")
	if !errors.Is(err, ErrTypeNotRegistered) {
		t.Errorf("expected ErrTypeNotRegistered, got %v", err)
	}
}

func TestRegistry_IsSubtypeOf(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.MustRegister("FinancialProduct", AsAbstract(), WithTable("financial_products"))
	reg.MustRegister("Loan", WithParent("FinancialProduct"))
	reg.MustRegister("Payment", WithParent("FinancialProduct"))

	if !reg.IsSubtypeOf("Loan", "FinancialProduct") {
		t.Error("expected Loan to be a subtype of FinancialProduct")
	}
	if reg.IsSubtypeOf("FinancialProduct", "Loan") {
		t.Error("expected FinancialProduct to not be a subtype of Loan")
	}
	if reg.IsSubtypeOf("Loan", "Payment") {
		t.Error("expected Loan to not be a subtype of Payment")
	}
	if reg.IsSubtypeOf("Unknown", "FinancialProduct") {
		t.Error("expected unknown type to not be a subtype")
	}
}

func TestRegistry_GetParent(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.MustRegister("FinancialProduct", AsAbstract(), WithTable("financial_products"))
	reg.MustRegister("Loan", WithParent("FinancialProduct"))

	parent, err := reg.GetParent("Loan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parent != "FinancialProduct" {
		t.Errorf("expected parent FinancialProduct, got %s", parent)
	}

	parent, err = reg.GetParent("FinancialProduct")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parent != "" {
		t.Errorf("expected empty parent for abstract type, got %s", parent)
	}
}

func TestRegistry_GetTable_Abstract(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.MustRegister("FinancialProduct", AsAbstract(), WithTable("financial_products"))

	table, err := reg.GetTable("FinancialProduct")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if table != "financial_products" {
		t.Errorf("expected financial_products, got %s", table)
	}
}

func TestRegistry_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		setup   func(*Registry)
		regName string
		opts    []Option
		wantErr error
	}{
		{
			name:    "duplicate registration",
			setup:   func(r *Registry) { r.MustRegister("Loan", AsAbstract(), WithTable("loans")) },
			regName: "Loan",
			opts:    []Option{AsAbstract(), WithTable("loans")},
			wantErr: ErrDuplicateType,
		},
		{
			name:    "abstract without table",
			regName: "Bad",
			opts:    []Option{AsAbstract()},
			wantErr: ErrAbstractRequiresTable,
		},
		{
			name:    "nested abstract",
			setup:   func(r *Registry) { r.MustRegister("Base", AsAbstract(), WithTable("base")) },
			regName: "Child",
			opts:    []Option{AsAbstract(), WithParent("Base"), WithTable("child")},
			wantErr: ErrNestedAbstract,
		},
		{
			name:    "parent not registered",
			regName: "Orphan",
			opts:    []Option{WithParent("Missing")},
			wantErr: ErrParentNotRegistered,
		},
		{
			name:    "parent not abstract",
			setup:   func(r *Registry) { r.MustRegister("Concrete") },
			regName: "Child",
			opts:    []Option{WithParent("Concrete")},
			wantErr: ErrParentNotAbstract,
		},
		{
			name: "concrete overrides table",
			setup: func(r *Registry) {
				r.MustRegister("Base", AsAbstract(), WithTable("base"))
			},
			regName: "Child",
			opts:    []Option{WithParent("Base"), WithTable("override")},
			wantErr: ErrCannotOverrideTable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reg := NewRegistry()
			if tt.setup != nil {
				tt.setup(reg)
			}

			err := reg.Register(tt.regName, tt.opts...)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestRegistry_MustRegister_Panics(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustRegister to panic on error")
		}
	}()

	reg.MustRegister("Bad", AsAbstract())
}

func TestRegistry_StandaloneConcreteType(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	reg.MustRegister("Standalone", WithTable("standalone_things"))

	if reg.IsAbstract("Standalone") {
		t.Error("expected Standalone to not be abstract")
	}

	table, err := reg.GetTable("Standalone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if table != "standalone_things" {
		t.Errorf("expected standalone_things, got %s", table)
	}

	types, err := reg.GetConcreteTypes("Standalone")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(types) != 1 || types[0] != "Standalone" {
		t.Errorf("expected [Standalone], got %v", types)
	}
}
