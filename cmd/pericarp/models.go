package main

// DomainModel represents the standardized internal model (Requirement 3.5, 6.6)
type DomainModel struct {
	ProjectName string                 `json:"project_name"`
	Entities    []Entity               `json:"entities"`
	Relations   []Relation             `json:"relations"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// Entity represents a domain entity with its properties
type Entity struct {
	Name       string                 `json:"name"`
	Properties []Property             `json:"properties"`
	Methods    []Method               `json:"methods,omitempty"`
	Events     []string               `json:"events,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// Property represents an entity property
type Property struct {
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	Required     bool                   `json:"required"`
	Tags         map[string]string      `json:"tags,omitempty"`
	DefaultValue string                 `json:"default_value,omitempty"`
	Validation   string                 `json:"validation,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// Method represents entity methods to be generated
type Method struct {
	Name           string      `json:"name"`
	Description    string      `json:"description,omitempty"`
	Parameters     []Parameter `json:"parameters,omitempty"`
	ReturnType     string      `json:"return_type,omitempty"`
	Implementation string      `json:"implementation,omitempty"`
}

// Parameter represents method parameters
type Parameter struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Relation represents relationships between entities (Requirement 3.5, 6.6)
type Relation struct {
	From        string                 `json:"from"`
	To          string                 `json:"to"`
	Type        RelationType           `json:"type"`
	Cardinality string                 `json:"cardinality"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// RelationType defines the types of relationships between entities
type RelationType string

const (
	OneToOne   RelationType = "one_to_one"
	OneToMany  RelationType = "one_to_many"
	ManyToMany RelationType = "many_to_many"
)

// GeneratedFile represents a generated code file
type GeneratedFile struct {
	Path     string                 `json:"path"`
	Content  string                 `json:"content"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ErrorType represents different types of CLI errors (Requirement 10.4, 10.5)
type ErrorType string

const (
	ValidationError ErrorType = "validation"
	ParseError      ErrorType = "parse"
	GenerationError ErrorType = "generation"
	FileSystemError ErrorType = "filesystem"
	NetworkError    ErrorType = "network"
	ArgumentError   ErrorType = "argument"
)

// CliError represents a CLI error with proper categorization (Requirement 10.1, 10.4, 10.5)
type CliError struct {
	Type    ErrorType `json:"type"`
	Message string    `json:"message"`
	Cause   error     `json:"cause,omitempty"`
	Code    int       `json:"code,omitempty"`
}

// Error implements the error interface
func (e *CliError) Error() string {
	if e.Cause != nil {
		return string(e.Type) + ": " + e.Message + " (caused by: " + e.Cause.Error() + ")"
	}
	return string(e.Type) + ": " + e.Message
}

// ExitCode returns the appropriate exit code for the error type
func (e *CliError) ExitCode() int {
	if e.Code != 0 {
		return e.Code
	}

	switch e.Type {
	case ArgumentError:
		return 2
	case ValidationError:
		return 3
	case ParseError:
		return 4
	case GenerationError:
		return 5
	case FileSystemError:
		return 6
	case NetworkError:
		return 7
	default:
		return 1
	}
}

// NewCliError creates a new CLI error with appropriate messaging (Requirement 10.1)
func NewCliError(errorType ErrorType, message string, cause error) *CliError {
	return &CliError{
		Type:    errorType,
		Message: message,
		Cause:   cause,
	}
}
