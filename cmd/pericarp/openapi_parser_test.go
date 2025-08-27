package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAPIParser_SupportedExtensions(t *testing.T) {
	parser := NewOpenAPIParser()
	extensions := parser.SupportedExtensions()

	expected := []string{".yaml", ".yml", ".json"}
	assert.Equal(t, expected, extensions)
}

func TestOpenAPIParser_FormatName(t *testing.T) {
	parser := NewOpenAPIParser()
	assert.Equal(t, "OpenAPI", parser.FormatName())
}

func TestOpenAPIParser_Validate(t *testing.T) {
	parser := NewOpenAPIParser()

	tests := []struct {
		name        string
		filePath    string
		setupFile   func(string) error
		wantErr     bool
		expectedErr ErrorType
	}{
		{
			name:        "empty file path",
			filePath:    "",
			wantErr:     true,
			expectedErr: ValidationError,
		},
		{
			name:        "non-existent file",
			filePath:    "non-existent.yaml",
			wantErr:     true,
			expectedErr: FileSystemError,
		},
		{
			name:        "unsupported extension",
			filePath:    "test.txt",
			setupFile:   func(path string) error { return os.WriteFile(path, []byte("test"), 0644) },
			wantErr:     true,
			expectedErr: ValidationError,
		},
		{
			name:     "valid yaml file",
			filePath: "test.yaml",
			setupFile: func(path string) error {
				validSpec := `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths: {}
components:
  schemas:
    TestEntity:
      type: object
      properties:
        id:
          type: string`
				return os.WriteFile(path, []byte(validSpec), 0644)
			},
			wantErr: false,
		},
		{
			name:     "valid yml file",
			filePath: "test.yml",
			setupFile: func(path string) error {
				validSpec := `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths: {}
components:
  schemas:
    TestEntity:
      type: object
      properties:
        id:
          type: string`
				return os.WriteFile(path, []byte(validSpec), 0644)
			},
			wantErr: false,
		},
		{
			name:     "valid json file",
			filePath: "test.json",
			setupFile: func(path string) error {
				validSpec := `{
  "openapi": "3.0.0",
  "info": {
    "title": "Test API",
    "version": "1.0.0"
  },
  "paths": {},
  "components": {
    "schemas": {
      "TestEntity": {
        "type": "object",
        "properties": {
          "id": {
            "type": "string"
          }
        }
      }
    }
  }
}`
				return os.WriteFile(path, []byte(validSpec), 0644)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			if tt.setupFile != nil {
				tempDir := t.TempDir()
				fullPath := filepath.Join(tempDir, tt.filePath)
				err := tt.setupFile(fullPath)
				require.NoError(t, err)
				tt.filePath = fullPath
			}

			// Test
			err := parser.Validate(tt.filePath)

			if tt.wantErr {
				assert.Error(t, err)
				if cliErr, ok := err.(*CliError); ok {
					assert.Equal(t, tt.expectedErr, cliErr.Type)
				}
			} else {
				assert.NoError(t, err)
			}

			// Cleanup handled by t.TempDir()
		})
	}
}

func TestOpenAPIParser_Parse_ValidSpec(t *testing.T) {
	parser := NewOpenAPIParser()

	// Create a valid OpenAPI specification
	validSpec := `openapi: 3.0.0
info:
  title: User Service API
  version: 1.0.0
  description: A simple user management service
paths: {}
components:
  schemas:
    User:
      type: object
      required:
        - email
        - name
      properties:
        id:
          type: string
          format: uuid
        email:
          type: string
          format: email
          minLength: 5
          maxLength: 100
        name:
          type: string
          minLength: 1
          maxLength: 50
        age:
          type: integer
          minimum: 0
          maximum: 150
        isActive:
          type: boolean
          default: true
        profile:
          $ref: '#/components/schemas/Profile'
        tags:
          type: array
          items:
            type: string
    Profile:
      type: object
      properties:
        bio:
          type: string
          maxLength: 500
        website:
          type: string
          format: uri
        createdAt:
          type: string
          format: date-time`

	// Write to temporary file
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "user-service.yaml")
	err := os.WriteFile(filePath, []byte(validSpec), 0644)
	require.NoError(t, err)

	// Parse the specification
	model, err := parser.Parse(filePath)
	require.NoError(t, err)
	require.NotNil(t, model)

	// Verify project name
	assert.Equal(t, "user-service-api", model.ProjectName)

	// Verify entities
	assert.Len(t, model.Entities, 2)

	// Find User entity
	var userEntity *Entity
	var profileEntity *Entity
	for i := range model.Entities {
		if model.Entities[i].Name == "User" {
			userEntity = &model.Entities[i]
		} else if model.Entities[i].Name == "Profile" {
			profileEntity = &model.Entities[i]
		}
	}

	require.NotNil(t, userEntity, "User entity should exist")
	require.NotNil(t, profileEntity, "Profile entity should exist")

	// Verify User entity properties
	assert.Len(t, userEntity.Properties, 7) // id, email, name, age, isActive, profile, tags

	// Check specific properties
	emailProp := findProperty(userEntity.Properties, "email")
	require.NotNil(t, emailProp, "email property should exist")
	assert.Equal(t, "string", emailProp.Type)
	assert.True(t, emailProp.Required, "email should be required")
	assert.Contains(t, emailProp.Validation, "email")
	assert.Contains(t, emailProp.Validation, "min=5")
	assert.Contains(t, emailProp.Validation, "max=100")

	nameProp := findProperty(userEntity.Properties, "name")
	require.NotNil(t, nameProp, "name property should exist")
	assert.True(t, nameProp.Required, "name should be required")

	ageProp := findProperty(userEntity.Properties, "age")
	require.NotNil(t, ageProp, "age property should exist")
	assert.Equal(t, "int", ageProp.Type)
	assert.False(t, ageProp.Required, "age should not be required")
	assert.Contains(t, ageProp.Validation, "min=0")
	assert.Contains(t, ageProp.Validation, "max=150")

	isActiveProp := findProperty(userEntity.Properties, "isActive")
	require.NotNil(t, isActiveProp, "isActive property should exist")
	assert.False(t, isActiveProp.Required, "isActive should not be required")
	assert.Equal(t, "true", isActiveProp.DefaultValue, "isActive should have default value")

	profileProp := findProperty(userEntity.Properties, "profile")
	require.NotNil(t, profileProp, "profile property should exist")
	assert.Equal(t, "Profile", profileProp.Type)

	tagsProp := findProperty(userEntity.Properties, "tags")
	require.NotNil(t, tagsProp, "tags property should exist")
	assert.Equal(t, "[]string", tagsProp.Type)

	// Verify events are generated
	assert.Contains(t, userEntity.Events, "UserCreated")
	assert.Contains(t, userEntity.Events, "UserUpdated")
	assert.Contains(t, userEntity.Events, "UserDeleted")

	// Verify relations
	assert.Len(t, model.Relations, 1)
	relation := model.Relations[0]
	assert.Equal(t, "User", relation.From)
	assert.Equal(t, "Profile", relation.To)
	assert.Equal(t, OneToOne, relation.Type)

	// Verify metadata
	assert.Equal(t, "openapi", model.Metadata["source_format"])
	assert.Equal(t, filePath, model.Metadata["source_file"])
}

func TestOpenAPIParser_Parse_ComplexTypes(t *testing.T) {
	parser := NewOpenAPIParser()

	complexSpec := `openapi: 3.0.0
info:
  title: Complex API
  version: 1.0.0
paths: {}
components:
  schemas:
    Order:
      type: object
      properties:
        id:
          type: string
          format: uuid
        items:
          type: array
          items:
            $ref: '#/components/schemas/OrderItem'
        customer:
          $ref: '#/components/schemas/Customer'
        total:
          type: number
          format: double
          minimum: 0
        createdAt:
          type: string
          format: date-time
    OrderItem:
      type: object
      properties:
        productId:
          type: string
          format: uuid
        quantity:
          type: integer
          format: int32
          minimum: 1
        price:
          type: number
          format: float
    Customer:
      type: object
      properties:
        id:
          type: string
          format: uuid
        name:
          type: string
        email:
          type: string
          format: email`

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "complex.yaml")
	err := os.WriteFile(filePath, []byte(complexSpec), 0644)
	require.NoError(t, err)

	model, err := parser.Parse(filePath)
	require.NoError(t, err)
	require.NotNil(t, model)

	// Verify all entities are created
	assert.Len(t, model.Entities, 3)

	// Find Order entity and verify complex relationships
	var orderEntity *Entity
	for i := range model.Entities {
		if model.Entities[i].Name == "Order" {
			orderEntity = &model.Entities[i]
			break
		}
	}
	require.NotNil(t, orderEntity)

	// Check array relationship
	itemsProp := findProperty(orderEntity.Properties, "items")
	require.NotNil(t, itemsProp)
	assert.Equal(t, "[]OrderItem", itemsProp.Type)

	// Check reference relationship
	customerProp := findProperty(orderEntity.Properties, "customer")
	require.NotNil(t, customerProp)
	assert.Equal(t, "Customer", customerProp.Type)

	// Verify relations include array relationship
	assert.Len(t, model.Relations, 2) // items array and customer reference

	// Find the array relationship
	var arrayRelation *Relation
	for i := range model.Relations {
		if model.Relations[i].Type == OneToMany {
			arrayRelation = &model.Relations[i]
			break
		}
	}
	require.NotNil(t, arrayRelation)
	assert.Equal(t, "Order", arrayRelation.From)
	assert.Equal(t, "OrderItem", arrayRelation.To)
}

func TestOpenAPIParser_Parse_InvalidSpecs(t *testing.T) {
	parser := NewOpenAPIParser()

	tests := []struct {
		name        string
		spec        string
		expectedErr ErrorType
		errorMsg    string
	}{
		{
			name:        "invalid YAML",
			spec:        "invalid: yaml: content: [",
			expectedErr: ParseError,
			errorMsg:    "file is not a valid OpenAPI specification",
		},
		{
			name: "missing paths",
			spec: `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
components:
  schemas:
    Test:
      type: object`,
			expectedErr: ParseError,
			errorMsg:    "OpenAPI specification validation failed",
		},
		{
			name: "empty schemas",
			spec: `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths: {}
components:
  schemas: {}`,
			expectedErr: ParseError,
			errorMsg:    "must contain at least one object schema",
		},
		{
			name: "non-object schemas only",
			spec: `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
paths: {}
components:
  schemas:
    StringType:
      type: string
    IntType:
      type: integer`,
			expectedErr: ParseError,
			errorMsg:    "must contain at least one object schema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			filePath := filepath.Join(tempDir, "test.yaml")
			err := os.WriteFile(filePath, []byte(tt.spec), 0644)
			require.NoError(t, err)

			model, err := parser.Parse(filePath)
			assert.Nil(t, model)
			assert.Error(t, err)

			if cliErr, ok := err.(*CliError); ok {
				assert.Equal(t, tt.expectedErr, cliErr.Type)
				assert.Contains(t, cliErr.Message, tt.errorMsg)
			}
		})
	}
}

func TestOpenAPIParser_TypeMapping(t *testing.T) {
	parser := NewOpenAPIParser()

	typeSpec := `openapi: 3.0.0
info:
  title: Type Test API
  version: 1.0.0
paths: {}
components:
  schemas:
    TypeTest:
      type: object
      properties:
        stringField:
          type: string
        uuidField:
          type: string
          format: uuid
        emailField:
          type: string
          format: email
        dateField:
          type: string
          format: date
        dateTimeField:
          type: string
          format: date-time
        intField:
          type: integer
        int32Field:
          type: integer
          format: int32
        int64Field:
          type: integer
          format: int64
        floatField:
          type: number
          format: float
        doubleField:
          type: number
          format: double
        numberField:
          type: number
        boolField:
          type: boolean
        byteField:
          type: string
          format: byte`

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "types.yaml")
	err := os.WriteFile(filePath, []byte(typeSpec), 0644)
	require.NoError(t, err)

	model, err := parser.Parse(filePath)
	require.NoError(t, err)
	require.Len(t, model.Entities, 1)

	entity := model.Entities[0]

	// Test type mappings
	typeTests := map[string]string{
		"stringField":   "string",
		"uuidField":     "uuid.UUID",
		"emailField":    "string",
		"dateField":     "time.Time",
		"dateTimeField": "time.Time",
		"intField":      "int",
		"int32Field":    "int32",
		"int64Field":    "int64",
		"floatField":    "float32",
		"doubleField":   "float64",
		"numberField":   "float64",
		"boolField":     "bool",
		"byteField":     "[]byte",
	}

	for propName, expectedType := range typeTests {
		prop := findProperty(entity.Properties, propName)
		require.NotNil(t, prop, "Property %s should exist", propName)
		assert.Equal(t, expectedType, prop.Type, "Property %s should have type %s", propName, expectedType)
	}
}

func TestOpenAPIParser_ValidationRules(t *testing.T) {
	parser := NewOpenAPIParser()

	validationSpec := `openapi: 3.0.0
info:
  title: Validation Test API
  version: 1.0.0
paths: {}
components:
  schemas:
    ValidationTest:
      type: object
      properties:
        minMaxString:
          type: string
          minLength: 5
          maxLength: 20
        patternString:
          type: string
          pattern: "^[A-Z][a-z]+$"
        emailString:
          type: string
          format: email
        minMaxNumber:
          type: number
          minimum: 0
          maximum: 100
        minMaxInteger:
          type: integer
          minimum: 1
          maximum: 10`

	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "validation.yaml")
	err := os.WriteFile(filePath, []byte(validationSpec), 0644)
	require.NoError(t, err)

	model, err := parser.Parse(filePath)
	require.NoError(t, err)
	require.Len(t, model.Entities, 1)

	entity := model.Entities[0]

	// Test validation rules
	minMaxStringProp := findProperty(entity.Properties, "minMaxString")
	require.NotNil(t, minMaxStringProp)
	assert.Contains(t, minMaxStringProp.Validation, "min=5")
	assert.Contains(t, minMaxStringProp.Validation, "max=20")

	patternStringProp := findProperty(entity.Properties, "patternString")
	require.NotNil(t, patternStringProp)
	assert.Contains(t, patternStringProp.Validation, "regexp=^[A-Z][a-z]+$")

	emailStringProp := findProperty(entity.Properties, "emailString")
	require.NotNil(t, emailStringProp)
	assert.Contains(t, emailStringProp.Validation, "email")

	minMaxNumberProp := findProperty(entity.Properties, "minMaxNumber")
	require.NotNil(t, minMaxNumberProp)
	assert.Contains(t, minMaxNumberProp.Validation, "min=0")
	assert.Contains(t, minMaxNumberProp.Validation, "max=100")
}

func TestOpenAPIParser_ProjectNameExtraction(t *testing.T) {
	parser := NewOpenAPIParser()

	tests := []struct {
		name         string
		spec         string
		filename     string
		expectedName string
	}{
		{
			name: "from title",
			spec: `openapi: 3.0.0
info:
  title: "User Management Service"
  version: 1.0.0
paths: {}
components:
  schemas:
    User:
      type: object
      properties:
        id:
          type: string`,
			filename:     "api.yaml",
			expectedName: "user-management-service",
		},
		{
			name: "from title with spaces and underscores",
			spec: `openapi: 3.0.0
info:
  title: "My_Cool API Service"
  version: 1.0.0
paths: {}
components:
  schemas:
    User:
      type: object
      properties:
        id:
          type: string`,
			filename:     "api.yaml",
			expectedName: "my-cool-api-service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			filePath := filepath.Join(tempDir, tt.filename)
			err := os.WriteFile(filePath, []byte(tt.spec), 0644)
			require.NoError(t, err)

			model, err := parser.Parse(filePath)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedName, model.ProjectName)
		})
	}
}

// Helper function to find a property by name
func findProperty(properties []Property, name string) *Property {
	for i := range properties {
		if properties[i].Name == name {
			return &properties[i]
		}
	}
	return nil
}
