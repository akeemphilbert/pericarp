package domain_test

import (
	"github.com/akeemphilbert/pericarp/pkg/domain"
	"testing"
)

// TestURL_FromURN_WithValidURN tests converting a valid URN to URL
func TestURL_FromURN_WithValidURN(t *testing.T) {
	// Arrange
	urn := new(domain.URN).With("square", "customer:CUSTOMER123")

	// Act
	result := new(domain.URL).WithNamespaces(map[string]string{
		"square":   "https://squareup.com",
		"customer": "/customers/",
	}).FromURN(*urn)

	// Assert
	if result == nil {
		t.Fatal("Expected FromURN to return a URL instance")
	}
	if !result.IsValid() {
		t.Errorf("Expected URL to be valid, got errors: %v", result.Errors())
	}

	// Verify the URL string matches expected format
	expectedURL := "https://squareup.com/customers/CUSTOMER123"
	actualURL := result.String()
	if actualURL != expectedURL {
		t.Errorf("Expected URL to be %q, got %q", expectedURL, actualURL)
	}
}

// TestURL_FromURN_WithEmptyURN tests handling of empty URN
func TestURL_FromURN_WithEmptyURN(t *testing.T) {
	// Arrange
	urn := domain.URN{} // Empty URN
	url := &domain.URL{}

	// Act
	result := url.FromURN(urn)

	// Assert
	if result.IsValid() {
		t.Error("Expected URL to be invalid with empty URN")
	}
	if len(result.Errors()) == 0 {
		t.Error("Expected at least one error for empty URN")
	}
}

// TestURL_FromURN_WithInvalidURNFormat tests handling of malformed URN
func TestURL_FromURN_WithInvalidURNFormat(t *testing.T) {
	// Arrange
	urn := domain.URN{Namespace: "", ID: ""} // Invalid format
	url := &domain.URL{}

	// Act
	result := url.FromURN(urn)

	// Assert
	if result.IsValid() {
		t.Error("Expected URL to be invalid with malformed URN")
	}
	errors := result.Errors()
	if len(errors) == 0 {
		t.Error("Expected errors for invalid URN format")
	}
}

// TestURL_WithNamespaces_WithNilMap tests handling of nil namespace map
func TestURL_WithNamespaces_WithNilMap(t *testing.T) {
	// Arrange
	url := &domain.URL{}

	// Act
	result := url.WithNamespaces(nil)

	// Assert
	if result == nil {
		t.Error("Expected WithNamespaces to return a URL instance even with nil map")
	}
	// Should handle nil gracefully without panicking
}

// TestURL_WithNamespaces_WithEmptyMap tests handling of empty namespace map
func TestURL_WithNamespaces_WithEmptyMap(t *testing.T) {
	// Arrange
	url := &domain.URL{}
	emptyMap := make(map[string]string)

	// Act
	result := url.WithNamespaces(emptyMap)

	// Assert
	if result == nil {
		t.Error("Expected WithNamespaces to return a URL instance")
	}
	// Should handle empty map gracefully
}

// TestURL_IsValid_WithErrors tests IsValid returns false when errors exist
func TestURL_IsValid_WithErrors(t *testing.T) {
	// Arrange
	urn := domain.URN{} // This should cause an error
	url := &domain.URL{}

	// Act
	url.FromURN(urn)

	// Assert
	if url.IsValid() {
		t.Error("Expected IsValid to return false when errors exist")
	}
	if len(url.Errors()) == 0 {
		t.Error("Expected errors to be present")
	}
}

// TestURL_Errors_ReturnsAllErrors tests error accumulation
func TestURL_Errors_ReturnsAllErrors(t *testing.T) {
	// Arrange
	url := &domain.URL{}
	emptyURN := domain.URN{}

	// Act - perform multiple operations that could add errors
	url.FromURN(emptyURN)
	url.FromURN(emptyURN) // Try again to potentially accumulate errors

	// Assert
	errors := url.Errors()
	if errors == nil {
		t.Error("Expected Errors() to return a slice, even if empty")
	}
	// Verify errors are accessible
	if len(errors) == 0 {
		t.Error("Expected at least one error to be accumulated")
	}
}

// TestURL_String_WithNoData tests String() on uninitialized URL
func TestURL_String_WithNoData(t *testing.T) {
	// Arrange
	url := &domain.URL{}

	// Act
	result := url.String()

	// Assert
	// Should not panic and return some string representation
	if result != "" {
		t.Logf("Uninitialized URL String() returned: %q", result)
	}
	// The current implementation returns empty string, which is acceptable
}
