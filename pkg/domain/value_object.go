package domain

import "github.com/segmentio/ksuid"

type URL struct {
	namespaces map[string]string
	errors     []error
	urnString  string
}

func (u *URL) WithNamespaces(namespaces map[string]string) *URL {
	u.namespaces = namespaces
	return u
}

// FromURN converts a URN to a URL based on predefined namespace mappings
func (u *URL) FromURN(urn URN) *URL {
	// Validate URN
	if urn.Namespace == "" || urn.ID == "" {
		u.errors = append(u.errors, &ValidationError{
			Field:   "URN",
			Message: "URN must have both namespace and ID",
		})
		return u
	}

	// Parse the URN ID to extract namespace:id pairs
	// For example: "customer:CUSTOMER123" -> namespace="customer", id="CUSTOMER123"
	var urlString string

	// Split the ID by ':' to get namespace and actual ID
	parts := splitNamespaceID(urn.ID)
	if len(parts) == 2 {
		subNamespace := parts[0]
		actualID := parts[1]

		// Build URL from namespace mappings
		if u.namespaces != nil {
			baseURL := u.namespaces[urn.Namespace]
			path := u.namespaces[subNamespace]
			urlString = baseURL + path + actualID
		}
	}

	u.urnString = urlString
	return u
}

// splitNamespaceID splits a string like "customer:CUSTOMER123" into ["customer", "CUSTOMER123"]
func splitNamespaceID(id string) []string {
	for i := 0; i < len(id); i++ {
		if id[i] == ':' {
			return []string{id[:i], id[i+1:]}
		}
	}
	return []string{id}
}

func (u *URL) String() string {
	return u.urnString
}

func (u *URL) Errors() []error {
	return u.errors
}

func (u *URL) IsValid() bool {
	return len(u.errors) == 0
}

// URN represents a Uniform Resource Name for Square entities
type URN struct {
	Namespace string
	ID        string
}

// With sets the namespace and ID for the URN
func (u *URN) With(namespace, id string) *URN {
	u.Namespace = namespace
	u.ID = id
	return u
}

// String returns the URN as a formatted string
func (u *URN) String() string {
	if u.ID == "" {
		u.ID = ksuid.New().String()
	}
	if u.Namespace == "" {
		u.Namespace = "square"
	}
	return "urn:" + u.Namespace + ":" + u.ID
}

// ValueObject defines the interface for domain value objects
type ValueObject interface {
	// Equals compares this value object with another for equality
	Equals(other ValueObject) bool

	// Validate ensures the value object is in a valid state
	Validate() error
}

// BaseValueObject provides common functionality for value objects
type BaseValueObject struct{}

// Validate provides a default validation implementation
func (bvo BaseValueObject) Validate() error {
	return nil
}
