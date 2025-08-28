package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// OpenAPIParser implements DomainParser for OpenAPI specifications
type OpenAPIParser struct{}

// NewOpenAPIParser creates a new OpenAPI parser
func NewOpenAPIParser() DomainParser {
	return &OpenAPIParser{}
}

// Parse processes an OpenAPI specification file and returns domain entities (Requirement 3.2, 3.5)
func (p *OpenAPIParser) Parse(filePath string) (*DomainModel, error) {
	if err := p.Validate(filePath); err != nil {
		return nil, err
	}

	// Load the OpenAPI specification using kin-openapi
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	// Get absolute path and convert to file URL
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, NewCliError(ParseError,
			fmt.Sprintf("failed to get absolute path: %s", filePath),
			err)
	}

	fileURL, err := url.Parse("file://" + filepath.ToSlash(absPath))
	if err != nil {
		return nil, NewCliError(ParseError,
			fmt.Sprintf("failed to create file URL: %s", filePath),
			err)
	}

	doc, err := loader.LoadFromURI(fileURL)
	if err != nil {
		return nil, NewCliError(ParseError,
			fmt.Sprintf("failed to load OpenAPI specification: %s", filePath),
			err)
	}

	// Validate the loaded document
	if err := doc.Validate(context.Background()); err != nil {
		return nil, NewCliError(ParseError,
			fmt.Sprintf("OpenAPI specification validation failed: %s", filePath),
			err)
	}

	return p.convertToDomainModel(doc, filePath)
}

// SupportedExtensions returns file extensions this parser handles
func (p *OpenAPIParser) SupportedExtensions() []string {
	return []string{".yaml", ".yml", ".json"}
}

// FormatName returns the human-readable name of this format
func (p *OpenAPIParser) FormatName() string {
	return "OpenAPI"
}

// Validate checks if the input file is valid for this parser (Requirement 10.3)
func (p *OpenAPIParser) Validate(filePath string) error {
	if filePath == "" {
		return NewCliError(ValidationError, "file path cannot be empty", nil)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return NewCliError(FileSystemError,
			fmt.Sprintf("OpenAPI file does not exist: %s", filePath),
			err)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	validExts := p.SupportedExtensions()
	for _, validExt := range validExts {
		if ext == validExt {
			// Do basic content validation to ensure it's actually an OpenAPI file
			return p.validateContent(filePath)
		}
	}

	return NewCliError(ValidationError,
		fmt.Sprintf("unsupported file extension for OpenAPI: %s (supported: %v)", ext, validExts),
		nil)
}

// validateContent performs basic validation of OpenAPI file content
func (p *OpenAPIParser) validateContent(filePath string) error {
	// Use kin-openapi for validation
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	// Get absolute path and convert to file URL
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return NewCliError(ParseError,
			fmt.Sprintf("failed to get absolute path: %s", filePath),
			err)
	}

	fileURL, err := url.Parse("file://" + filepath.ToSlash(absPath))
	if err != nil {
		return NewCliError(ParseError,
			fmt.Sprintf("failed to create file URL: %s", filePath),
			err)
	}

	doc, err := loader.LoadFromURI(fileURL)
	if err != nil {
		return NewCliError(ParseError,
			fmt.Sprintf("file is not a valid OpenAPI specification: %s", filePath),
			err)
	}

	// Validate the loaded document
	if err := doc.Validate(context.Background()); err != nil {
		return NewCliError(ParseError,
			fmt.Sprintf("OpenAPI specification validation failed: %s", filePath),
			err)
	}

	// Check if there are any aggregate schemas (object schemas with x-aggregate: true)
	if doc.Components == nil || doc.Components.Schemas == nil {
		return NewCliError(ParseError,
			fmt.Sprintf("OpenAPI file must contain components.schemas section: %s", filePath),
			nil)
	}

	hasAggregateSchema := false
	for _, schemaRef := range doc.Components.Schemas {
		if schemaRef.Value != nil && schemaRef.Value.Type != nil && schemaRef.Value.Type.Is("object") {
			if p.isAggregateSchema(schemaRef.Value) {
				hasAggregateSchema = true
				break
			}
		}
	}

	if !hasAggregateSchema {
		return NewCliError(ParseError,
			fmt.Sprintf("OpenAPI file must contain at least one object schema with x-aggregate: true extension: %s", filePath),
			nil)
	}

	return nil
}

// convertToDomainModel converts OpenAPI spec to internal domain model
func (p *OpenAPIParser) convertToDomainModel(doc *openapi3.T, filePath string) (*DomainModel, error) {
	if doc.Components == nil || doc.Components.Schemas == nil {
		return nil, NewCliError(ParseError,
			"OpenAPI specification must contain components.schemas section",
			nil)
	}

	projectName := p.extractProjectName(doc, filePath)
	entities := make([]Entity, 0)
	relations := make([]Relation, 0)

	// Convert each schema to an entity, but only if it has x-aggregate: true
	for schemaName, schemaRef := range doc.Components.Schemas {
		if schemaRef.Value != nil && schemaRef.Value.Type != nil && schemaRef.Value.Type.Is("object") {
			// Check if schema has x-aggregate extension with value true
			if !p.isAggregateSchema(schemaRef.Value) {
				continue // Skip schemas that are not marked as aggregates
			}

			entity, entityRelations, err := p.convertSchemaToEntity(schemaName, schemaRef.Value, doc.Components.Schemas)
			if err != nil {
				return nil, NewCliError(ParseError,
					fmt.Sprintf("failed to convert schema '%s' to entity", schemaName),
					err)
			}
			entities = append(entities, entity)
			relations = append(relations, entityRelations...)
		}
	}

	if len(entities) == 0 {
		return nil, NewCliError(ParseError,
			"no schemas with x-aggregate: true extension found in OpenAPI specification",
			nil)
	}

	return &DomainModel{
		ProjectName: projectName,
		Entities:    entities,
		Relations:   relations,
		Metadata: map[string]interface{}{
			"source_format": "openapi",
			"source_file":   filePath,
			"openapi_info":  doc.Info,
		},
	}, nil
}

// convertSchemaToEntity converts an OpenAPI schema to a domain entity
func (p *OpenAPIParser) convertSchemaToEntity(name string, schema *openapi3.Schema, allSchemas openapi3.Schemas) (Entity, []Relation, error) {
	entity := Entity{
		Name:       name,
		Properties: make([]Property, 0),
		Methods:    make([]Method, 0),
		Events:     []string{name + "Created", name + "Updated", name + "Deleted"},
		Metadata: map[string]interface{}{
			"openapi_schema": schema,
		},
	}

	relations := make([]Relation, 0)

	// Convert properties
	for propName, propSchemaRef := range schema.Properties {
		if propSchemaRef.Value == nil {
			continue // Skip invalid property references
		}

		property, propRelations, err := p.convertProperty(propName, propSchemaRef, name, allSchemas)
		if err != nil {
			return entity, relations, fmt.Errorf("failed to convert property '%s': %w", propName, err)
		}

		// Set required field based on schema's required array
		property.Required = p.isPropertyRequired(propName, schema.Required)

		entity.Properties = append(entity.Properties, property)
		relations = append(relations, propRelations...)
	}

	return entity, relations, nil
}

// convertProperty converts an OpenAPI property to a domain property
func (p *OpenAPIParser) convertProperty(name string, schemaRef *openapi3.SchemaRef, entityName string, allSchemas openapi3.Schemas) (Property, []Relation, error) {
	property := Property{
		Name:     name,
		Required: false, // Will be set based on parent schema's required array
		Tags:     make(map[string]string),
		Metadata: make(map[string]interface{}),
	}

	relations := make([]Relation, 0)

	// Handle references first
	if schemaRef.Ref != "" {
		refType, relation := p.handleReference(schemaRef.Ref, entityName, name)
		property.Type = refType
		if relation != nil {
			relations = append(relations, *relation)
		}
	} else if schemaRef.Value != nil {
		schema := schemaRef.Value

		// Handle different property types
		if schema.Type != nil {
			if schema.Type.Is("string") {
				property.Type = p.mapStringType(schema)
				p.addStringValidation(&property, schema)
			} else if schema.Type.Is("integer") {
				property.Type = p.mapIntegerType(schema)
				p.addNumericValidation(&property, schema)
			} else if schema.Type.Is("number") {
				property.Type = p.mapNumberType(schema)
				p.addNumericValidation(&property, schema)
			} else if schema.Type.Is("boolean") {
				property.Type = "bool"
			} else if schema.Type.Is("array") {
				arrayType, arrayRelations, err := p.handleArrayType(schema, entityName, allSchemas)
				if err != nil {
					return property, relations, err
				}
				property.Type = arrayType
				relations = append(relations, arrayRelations...)
			} else if schema.Type.Is("object") {
				// Handle nested objects (inline objects become map[string]interface{})
				property.Type = "map[string]interface{}"
			} else {
				property.Type = "interface{}"
			}
		} else {
			property.Type = "interface{}"
		}

		// Set default value if present
		if schema.Default != nil {
			property.DefaultValue = fmt.Sprintf("%v", schema.Default)
		}

		// Store OpenAPI metadata
		property.Metadata["openapi_schema"] = schema
	} else {
		property.Type = "interface{}"
	}

	// Add JSON tags
	property.Tags["json"] = name

	// Add validation tags if present
	if property.Validation != "" {
		property.Tags["validate"] = property.Validation
	}

	return property, relations, nil
}

// mapStringType maps OpenAPI string types to Go types
func (p *OpenAPIParser) mapStringType(schema *openapi3.Schema) string {
	switch schema.Format {
	case "uuid":
		return "ksuid.KSUID"
	case "date":
		return "time.Time"
	case "date-time":
		return "time.Time"
	case "email":
		return "string"
	case "uri":
		return "string"
	case "byte":
		return "[]byte"
	default:
		return "string"
	}
}

// mapIntegerType maps OpenAPI integer types to Go types
func (p *OpenAPIParser) mapIntegerType(schema *openapi3.Schema) string {
	switch schema.Format {
	case "int32":
		return "int32"
	case "int64":
		return "int64"
	default:
		return "int"
	}
}

// mapNumberType maps OpenAPI number types to Go types
func (p *OpenAPIParser) mapNumberType(schema *openapi3.Schema) string {
	switch schema.Format {
	case "float":
		return "float32"
	case "double":
		return "float64"
	default:
		return "float64"
	}
}

// addStringValidation adds validation rules for string properties
func (p *OpenAPIParser) addStringValidation(property *Property, schema *openapi3.Schema) {
	validations := make([]string, 0)

	if schema.MinLength > 0 {
		validations = append(validations, fmt.Sprintf("min=%d", schema.MinLength))
	}

	if schema.MaxLength != nil && *schema.MaxLength > 0 {
		validations = append(validations, fmt.Sprintf("max=%d", *schema.MaxLength))
	}

	if schema.Pattern != "" {
		validations = append(validations, fmt.Sprintf("regexp=%s", schema.Pattern))
	}

	if schema.Format == "email" {
		validations = append(validations, "email")
	}

	if schema.Format == "uri" {
		validations = append(validations, "uri")
	}

	if len(validations) > 0 {
		property.Validation = strings.Join(validations, ",")
	}
}

// addNumericValidation adds validation rules for numeric properties
func (p *OpenAPIParser) addNumericValidation(property *Property, schema *openapi3.Schema) {
	validations := make([]string, 0)

	if schema.Min != nil {
		validations = append(validations, fmt.Sprintf("min=%v", *schema.Min))
	}

	if schema.Max != nil {
		validations = append(validations, fmt.Sprintf("max=%v", *schema.Max))
	}

	if len(validations) > 0 {
		property.Validation = strings.Join(validations, ",")
	}
}

// handleArrayType processes array type properties
func (p *OpenAPIParser) handleArrayType(schema *openapi3.Schema, entityName string, allSchemas openapi3.Schemas) (string, []Relation, error) {
	relations := make([]Relation, 0)

	if schema.Items == nil || schema.Items.Value == nil {
		return "[]interface{}", relations, nil
	}

	itemSchema := schema.Items.Value

	// Handle reference to another schema
	if schema.Items.Ref != "" {
		refType, relation := p.handleReference(schema.Items.Ref, entityName, "items")
		if relation != nil {
			relation.Type = OneToMany
			relations = append(relations, *relation)
		}
		return "[]" + refType, relations, nil
	}

	// Handle primitive types
	if itemSchema.Type != nil {
		if itemSchema.Type.Is("string") {
			return "[]string", relations, nil
		} else if itemSchema.Type.Is("integer") {
			return "[]int", relations, nil
		} else if itemSchema.Type.Is("number") {
			return "[]float64", relations, nil
		} else if itemSchema.Type.Is("boolean") {
			return "[]bool", relations, nil
		}
	}
	return "[]interface{}", relations, nil
}

// handleReference processes OpenAPI references and creates relationships
func (p *OpenAPIParser) handleReference(ref, fromEntity, propertyName string) (string, *Relation) {
	// Extract the referenced type name from #/components/schemas/TypeName
	parts := strings.Split(ref, "/")
	if len(parts) < 1 {
		return "interface{}", nil
	}

	refType := parts[len(parts)-1]

	// Create a relationship
	relation := &Relation{
		From:        fromEntity,
		To:          refType,
		Type:        OneToOne, // Default to one-to-one, can be overridden
		Cardinality: "1:1",
		Metadata: map[string]interface{}{
			"property_name": propertyName,
			"reference":     ref,
		},
	}

	return refType, relation
}

// isPropertyRequired checks if a property is in the required array
func (p *OpenAPIParser) isPropertyRequired(propName string, required []string) bool {
	for _, req := range required {
		if req == propName {
			return true
		}
	}
	return false
}

// isAggregateSchema checks if a schema has the x-aggregate extension with value true
func (p *OpenAPIParser) isAggregateSchema(schema *openapi3.Schema) bool {
	if schema.Extensions == nil {
		return false
	}

	xAggregate, exists := schema.Extensions["x-aggregate"]
	if !exists {
		return false
	}

	// Check if the value is true (can be boolean true or string "true")
	switch v := xAggregate.(type) {
	case bool:
		return v
	case string:
		return strings.ToLower(v) == "true"
	default:
		return false
	}
}

// extractProjectName extracts project name from OpenAPI spec or file path
func (p *OpenAPIParser) extractProjectName(doc *openapi3.T, filePath string) string {
	if doc.Info != nil && doc.Info.Title != "" {
		// Convert title to valid Go module name
		title := strings.ToLower(doc.Info.Title)
		title = strings.ReplaceAll(title, " ", "-")
		title = strings.ReplaceAll(title, "_", "-")
		return title
	}

	// Fallback to filename without extension
	filename := filepath.Base(filePath)
	ext := filepath.Ext(filename)
	return strings.TrimSuffix(filename, ext)
}
