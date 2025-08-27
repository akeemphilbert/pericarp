package main

import (
	"encoding/json"
	"testing"
)

func TestDomainModel(t *testing.T) {
	tests := []struct {
		name     string
		model    DomainModel
		expected DomainModel
	}{
		{
			name: "create domain model with entities and relations",
			model: DomainModel{
				ProjectName: "user-service",
				Entities: []Entity{
					{
						Name: "User",
						Properties: []Property{
							{Name: "id", Type: "uuid.UUID", Required: true},
							{Name: "email", Type: "string", Required: true},
						},
					},
				},
				Relations: []Relation{
					{
						From:        "User",
						To:          "Profile",
						Type:        OneToOne,
						Cardinality: "1:1",
					},
				},
				Metadata: map[string]interface{}{
					"version": "1.0",
				},
			},
			expected: DomainModel{
				ProjectName: "user-service",
				Entities: []Entity{
					{
						Name: "User",
						Properties: []Property{
							{Name: "id", Type: "uuid.UUID", Required: true},
							{Name: "email", Type: "string", Required: true},
						},
					},
				},
				Relations: []Relation{
					{
						From:        "User",
						To:          "Profile",
						Type:        OneToOne,
						Cardinality: "1:1",
					},
				},
				Metadata: map[string]interface{}{
					"version": "1.0",
				},
			},
		},
		{
			name: "empty domain model",
			model: DomainModel{
				ProjectName: "empty-project",
				Entities:    []Entity{},
				Relations:   []Relation{},
			},
			expected: DomainModel{
				ProjectName: "empty-project",
				Entities:    []Entity{},
				Relations:   []Relation{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.model.ProjectName != tt.expected.ProjectName {
				t.Errorf("ProjectName = %v, want %v", tt.model.ProjectName, tt.expected.ProjectName)
			}
			if len(tt.model.Entities) != len(tt.expected.Entities) {
				t.Errorf("Entities length = %v, want %v", len(tt.model.Entities), len(tt.expected.Entities))
			}
			if len(tt.model.Relations) != len(tt.expected.Relations) {
				t.Errorf("Relations length = %v, want %v", len(tt.model.Relations), len(tt.expected.Relations))
			}
		})
	}
}

func TestEntity(t *testing.T) {
	tests := []struct {
		name     string
		entity   Entity
		expected Entity
	}{
		{
			name: "entity with properties and methods",
			entity: Entity{
				Name: "User",
				Properties: []Property{
					{Name: "id", Type: "uuid.UUID", Required: true},
					{Name: "email", Type: "string", Required: true},
					{Name: "name", Type: "string", Required: false},
				},
				Methods: []Method{
					{
						Name:        "UpdateEmail",
						Description: "Updates user email",
						Parameters: []Parameter{
							{Name: "email", Type: "string"},
						},
						ReturnType: "error",
					},
				},
				Events: []string{"UserCreated", "UserUpdated"},
				Metadata: map[string]interface{}{
					"table": "users",
				},
			},
			expected: Entity{
				Name: "User",
				Properties: []Property{
					{Name: "id", Type: "uuid.UUID", Required: true},
					{Name: "email", Type: "string", Required: true},
					{Name: "name", Type: "string", Required: false},
				},
				Methods: []Method{
					{
						Name:        "UpdateEmail",
						Description: "Updates user email",
						Parameters: []Parameter{
							{Name: "email", Type: "string"},
						},
						ReturnType: "error",
					},
				},
				Events: []string{"UserCreated", "UserUpdated"},
				Metadata: map[string]interface{}{
					"table": "users",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.entity.Name != tt.expected.Name {
				t.Errorf("Name = %v, want %v", tt.entity.Name, tt.expected.Name)
			}
			if len(tt.entity.Properties) != len(tt.expected.Properties) {
				t.Errorf("Properties length = %v, want %v", len(tt.entity.Properties), len(tt.expected.Properties))
			}
			if len(tt.entity.Methods) != len(tt.expected.Methods) {
				t.Errorf("Methods length = %v, want %v", len(tt.entity.Methods), len(tt.expected.Methods))
			}
			if len(tt.entity.Events) != len(tt.expected.Events) {
				t.Errorf("Events length = %v, want %v", len(tt.entity.Events), len(tt.expected.Events))
			}
		})
	}
}

func TestProperty(t *testing.T) {
	tests := []struct {
		name     string
		property Property
		expected Property
	}{
		{
			name: "required property with validation",
			property: Property{
				Name:         "email",
				Type:         "string",
				Required:     true,
				Tags:         map[string]string{"json": "email", "validate": "email"},
				DefaultValue: "",
				Validation:   "email",
				Metadata:     map[string]interface{}{"column": "email_address"},
			},
			expected: Property{
				Name:         "email",
				Type:         "string",
				Required:     true,
				Tags:         map[string]string{"json": "email", "validate": "email"},
				DefaultValue: "",
				Validation:   "email",
				Metadata:     map[string]interface{}{"column": "email_address"},
			},
		},
		{
			name: "optional property with default value",
			property: Property{
				Name:         "isActive",
				Type:         "bool",
				Required:     false,
				DefaultValue: "true",
				Tags:         map[string]string{"json": "is_active"},
			},
			expected: Property{
				Name:         "isActive",
				Type:         "bool",
				Required:     false,
				DefaultValue: "true",
				Tags:         map[string]string{"json": "is_active"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.property.Name != tt.expected.Name {
				t.Errorf("Name = %v, want %v", tt.property.Name, tt.expected.Name)
			}
			if tt.property.Type != tt.expected.Type {
				t.Errorf("Type = %v, want %v", tt.property.Type, tt.expected.Type)
			}
			if tt.property.Required != tt.expected.Required {
				t.Errorf("Required = %v, want %v", tt.property.Required, tt.expected.Required)
			}
			if tt.property.DefaultValue != tt.expected.DefaultValue {
				t.Errorf("DefaultValue = %v, want %v", tt.property.DefaultValue, tt.expected.DefaultValue)
			}
		})
	}
}

func TestRelation(t *testing.T) {
	tests := []struct {
		name     string
		relation Relation
		expected Relation
	}{
		{
			name: "one-to-one relation",
			relation: Relation{
				From:        "User",
				To:          "Profile",
				Type:        OneToOne,
				Cardinality: "1:1",
				Metadata:    map[string]interface{}{"foreign_key": "user_id"},
			},
			expected: Relation{
				From:        "User",
				To:          "Profile",
				Type:        OneToOne,
				Cardinality: "1:1",
				Metadata:    map[string]interface{}{"foreign_key": "user_id"},
			},
		},
		{
			name: "one-to-many relation",
			relation: Relation{
				From:        "User",
				To:          "Order",
				Type:        OneToMany,
				Cardinality: "1:N",
			},
			expected: Relation{
				From:        "User",
				To:          "Order",
				Type:        OneToMany,
				Cardinality: "1:N",
			},
		},
		{
			name: "many-to-many relation",
			relation: Relation{
				From:        "User",
				To:          "Role",
				Type:        ManyToMany,
				Cardinality: "N:M",
				Metadata:    map[string]interface{}{"join_table": "user_roles"},
			},
			expected: Relation{
				From:        "User",
				To:          "Role",
				Type:        ManyToMany,
				Cardinality: "N:M",
				Metadata:    map[string]interface{}{"join_table": "user_roles"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.relation.From != tt.expected.From {
				t.Errorf("From = %v, want %v", tt.relation.From, tt.expected.From)
			}
			if tt.relation.To != tt.expected.To {
				t.Errorf("To = %v, want %v", tt.relation.To, tt.expected.To)
			}
			if tt.relation.Type != tt.expected.Type {
				t.Errorf("Type = %v, want %v", tt.relation.Type, tt.expected.Type)
			}
			if tt.relation.Cardinality != tt.expected.Cardinality {
				t.Errorf("Cardinality = %v, want %v", tt.relation.Cardinality, tt.expected.Cardinality)
			}
		})
	}
}

func TestRelationType(t *testing.T) {
	tests := []struct {
		name     string
		relType  RelationType
		expected string
	}{
		{
			name:     "one-to-one relation type",
			relType:  OneToOne,
			expected: "one_to_one",
		},
		{
			name:     "one-to-many relation type",
			relType:  OneToMany,
			expected: "one_to_many",
		},
		{
			name:     "many-to-many relation type",
			relType:  ManyToMany,
			expected: "many_to_many",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.relType) != tt.expected {
				t.Errorf("RelationType = %v, want %v", string(tt.relType), tt.expected)
			}
		})
	}
}

func TestMethod(t *testing.T) {
	tests := []struct {
		name     string
		method   Method
		expected Method
	}{
		{
			name: "method with parameters and return type",
			method: Method{
				Name:        "UpdateEmail",
				Description: "Updates the user's email address",
				Parameters: []Parameter{
					{Name: "email", Type: "string"},
					{Name: "ctx", Type: "context.Context"},
				},
				ReturnType:     "error",
				Implementation: "// Implementation will be generated",
			},
			expected: Method{
				Name:        "UpdateEmail",
				Description: "Updates the user's email address",
				Parameters: []Parameter{
					{Name: "email", Type: "string"},
					{Name: "ctx", Type: "context.Context"},
				},
				ReturnType:     "error",
				Implementation: "// Implementation will be generated",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.method.Name != tt.expected.Name {
				t.Errorf("Name = %v, want %v", tt.method.Name, tt.expected.Name)
			}
			if tt.method.Description != tt.expected.Description {
				t.Errorf("Description = %v, want %v", tt.method.Description, tt.expected.Description)
			}
			if len(tt.method.Parameters) != len(tt.expected.Parameters) {
				t.Errorf("Parameters length = %v, want %v", len(tt.method.Parameters), len(tt.expected.Parameters))
			}
			if tt.method.ReturnType != tt.expected.ReturnType {
				t.Errorf("ReturnType = %v, want %v", tt.method.ReturnType, tt.expected.ReturnType)
			}
		})
	}
}

func TestGeneratedFile(t *testing.T) {
	tests := []struct {
		name     string
		file     GeneratedFile
		expected GeneratedFile
	}{
		{
			name: "generated file with content and metadata",
			file: GeneratedFile{
				Path:    "internal/domain/user.go",
				Content: "package domain\n\ntype User struct {\n\tID string\n}",
				Metadata: map[string]interface{}{
					"template": "entity.go.tmpl",
					"entity":   "User",
				},
			},
			expected: GeneratedFile{
				Path:    "internal/domain/user.go",
				Content: "package domain\n\ntype User struct {\n\tID string\n}",
				Metadata: map[string]interface{}{
					"template": "entity.go.tmpl",
					"entity":   "User",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.file.Path != tt.expected.Path {
				t.Errorf("Path = %v, want %v", tt.file.Path, tt.expected.Path)
			}
			if tt.file.Content != tt.expected.Content {
				t.Errorf("Content = %v, want %v", tt.file.Content, tt.expected.Content)
			}
		})
	}
}

func TestDomainModelJSONSerialization(t *testing.T) {
	model := DomainModel{
		ProjectName: "test-project",
		Entities: []Entity{
			{
				Name: "User",
				Properties: []Property{
					{Name: "id", Type: "string", Required: true},
				},
			},
		},
		Relations: []Relation{
			{
				From:        "User",
				To:          "Profile",
				Type:        OneToOne,
				Cardinality: "1:1",
			},
		},
		Metadata: map[string]interface{}{
			"version": "1.0",
		},
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(model)
	if err != nil {
		t.Errorf("Failed to marshal DomainModel to JSON: %v", err)
	}

	// Test JSON unmarshaling
	var unmarshaled DomainModel
	err = json.Unmarshal(jsonData, &unmarshaled)
	if err != nil {
		t.Errorf("Failed to unmarshal DomainModel from JSON: %v", err)
	}

	// Verify the unmarshaled data
	if unmarshaled.ProjectName != model.ProjectName {
		t.Errorf("ProjectName = %v, want %v", unmarshaled.ProjectName, model.ProjectName)
	}
	if len(unmarshaled.Entities) != len(model.Entities) {
		t.Errorf("Entities length = %v, want %v", len(unmarshaled.Entities), len(model.Entities))
	}
	if len(unmarshaled.Relations) != len(model.Relations) {
		t.Errorf("Relations length = %v, want %v", len(unmarshaled.Relations), len(model.Relations))
	}
}
