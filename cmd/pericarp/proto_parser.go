package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"google.golang.org/protobuf/types/descriptorpb"
)

// ProtoParser implements DomainParser for Protocol Buffer definitions
type ProtoParser struct{}

// NewProtoParser creates a new Protocol Buffer parser
func NewProtoParser() DomainParser {
	return &ProtoParser{}
}

// Parse processes a Protocol Buffer file and returns domain entities (Requirement 3.3, 3.5)
func (p *ProtoParser) Parse(filePath string) (*DomainModel, error) {
	if err := p.Validate(filePath); err != nil {
		return nil, err
	}

	// Parse the proto file using protoreflect
	parser := &protoparse.Parser{
		ImportPaths:                     []string{filepath.Dir(filePath)},
		IncludeSourceCodeInfo:           true,
		InterpretOptionsInUnlinkedFiles: true,
	}

	fileDescriptors, err := parser.ParseFiles(filepath.Base(filePath))
	if err != nil {
		return nil, NewCliError(ParseError,
			fmt.Sprintf("failed to parse Protocol Buffer file: %s", filePath),
			err)
	}

	if len(fileDescriptors) == 0 {
		return nil, NewCliError(ParseError,
			fmt.Sprintf("no valid Protocol Buffer definitions found in: %s", filePath),
			nil)
	}

	return p.convertToDomainModel(fileDescriptors[0], filePath)
}

// SupportedExtensions returns file extensions this parser handles
func (p *ProtoParser) SupportedExtensions() []string {
	return []string{".proto"}
}

// FormatName returns the human-readable name of this format
func (p *ProtoParser) FormatName() string {
	return "Protocol Buffers"
}

// Validate checks if the input file is valid for this parser (Requirement 10.3)
func (p *ProtoParser) Validate(filePath string) error {
	if filePath == "" {
		return NewCliError(ValidationError, "file path cannot be empty", nil)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return NewCliError(FileSystemError,
			fmt.Sprintf("Protocol Buffer file does not exist: %s", filePath),
			err)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if ext != ".proto" {
		return NewCliError(ValidationError,
			fmt.Sprintf("unsupported file extension for Protocol Buffers: %s (expected: .proto)", ext),
			nil)
	}

	// Do basic content validation to ensure it's actually a proto file
	return p.validateContent(filePath)
}

// validateContent performs basic validation of Protocol Buffer file content
func (p *ProtoParser) validateContent(filePath string) error {
	// Try to parse the file to validate syntax
	parser := &protoparse.Parser{
		ImportPaths:                     []string{filepath.Dir(filePath)},
		IncludeSourceCodeInfo:           true,
		InterpretOptionsInUnlinkedFiles: true,
	}

	_, err := parser.ParseFiles(filepath.Base(filePath))
	if err != nil {
		return NewCliError(ParseError,
			fmt.Sprintf("file is not a valid Protocol Buffer definition: %s", filePath),
			err)
	}

	return nil
}

// convertToDomainModel converts Protocol Buffer file descriptor to internal domain model
func (p *ProtoParser) convertToDomainModel(fileDesc *desc.FileDescriptor, filePath string) (*DomainModel, error) {
	projectName := p.extractProjectName(fileDesc, filePath)
	entities := make([]Entity, 0)
	relations := make([]Relation, 0)

	// Convert each message to an entity
	for _, msgDesc := range fileDesc.GetMessageTypes() {
		// Skip request/response messages as they are typically DTOs, not domain entities
		if p.isRequestResponseMessage(msgDesc.GetName()) {
			continue
		}

		entity, entityRelations, err := p.convertMessageToEntity(msgDesc)
		if err != nil {
			return nil, NewCliError(ParseError,
				fmt.Sprintf("failed to convert message '%s' to entity", msgDesc.GetName()),
				err)
		}
		entities = append(entities, entity)
		relations = append(relations, entityRelations...)
	}

	if len(entities) == 0 {
		return nil, NewCliError(ParseError,
			"no valid domain entities found in Protocol Buffer file (only request/response messages found)",
			nil)
	}

	return &DomainModel{
		ProjectName: projectName,
		Entities:    entities,
		Relations:   relations,
		Metadata: map[string]interface{}{
			"source_format": "protobuf",
			"source_file":   filePath,
			"package":       fileDesc.GetPackage(),
			"go_package":    p.extractGoPackage(fileDesc),
			"services":      p.extractServiceInfo(fileDesc),
		},
	}, nil
}

// convertMessageToEntity converts a Protocol Buffer message to a domain entity
func (p *ProtoParser) convertMessageToEntity(msgDesc *desc.MessageDescriptor) (Entity, []Relation, error) {
	entity := Entity{
		Name:       msgDesc.GetName(),
		Properties: make([]Property, 0),
		Methods:    make([]Method, 0),
		Events:     []string{msgDesc.GetName() + "Created", msgDesc.GetName() + "Updated", msgDesc.GetName() + "Deleted"},
		Metadata: map[string]interface{}{
			"proto_message": msgDesc.GetName(),
			"proto_package": msgDesc.GetFile().GetPackage(),
		},
	}

	relations := make([]Relation, 0)

	// Convert fields to properties
	for _, fieldDesc := range msgDesc.GetFields() {
		property, fieldRelations, err := p.convertFieldToProperty(fieldDesc, msgDesc.GetName())
		if err != nil {
			return entity, relations, fmt.Errorf("failed to convert field '%s': %w", fieldDesc.GetName(), err)
		}

		entity.Properties = append(entity.Properties, property)
		relations = append(relations, fieldRelations...)
	}

	return entity, relations, nil
}

// convertFieldToProperty converts a Protocol Buffer field to a domain property
func (p *ProtoParser) convertFieldToProperty(fieldDesc *desc.FieldDescriptor, entityName string) (Property, []Relation, error) {
	property := Property{
		Name:     p.convertFieldName(fieldDesc.GetName()),
		Required: !fieldDesc.IsRepeated(), // Proto3 fields are optional by default
		Tags:     make(map[string]string),
		Metadata: map[string]interface{}{
			"proto_field":  fieldDesc.GetName(),
			"proto_number": fieldDesc.GetNumber(),
			"proto_type":   fieldDesc.GetType().String(),
		},
	}

	relations := make([]Relation, 0)

	// Handle different field types
	if fieldDesc.IsRepeated() {
		// Handle repeated fields (arrays/slices)
		baseType, fieldRelations, err := p.mapProtoType(fieldDesc, entityName)
		if err != nil {
			return property, relations, err
		}
		property.Type = "[]" + baseType
		relations = append(relations, fieldRelations...)

		// For repeated message types, create one-to-many relationship
		if fieldDesc.GetMessageType() != nil && !p.isRequestResponseMessage(fieldDesc.GetMessageType().GetName()) {
			relation := Relation{
				From:        entityName,
				To:          fieldDesc.GetMessageType().GetName(),
				Type:        OneToMany,
				Cardinality: "1:*",
				Metadata: map[string]interface{}{
					"field_name":  fieldDesc.GetName(),
					"proto_field": true,
					"repeated":    true,
				},
			}
			relations = append(relations, relation)
		}
	} else if fieldDesc.IsMap() {
		// Handle map fields - for now, simplify to map[string]interface{}
		property.Type = "map[string]interface{}"
	} else {
		// Handle regular fields
		fieldType, fieldRelations, err := p.mapProtoType(fieldDesc, entityName)
		if err != nil {
			return property, relations, err
		}
		property.Type = fieldType
		relations = append(relations, fieldRelations...)

		// For message types, create one-to-one relationship
		if fieldDesc.GetMessageType() != nil && !p.isRequestResponseMessage(fieldDesc.GetMessageType().GetName()) {
			relation := Relation{
				From:        entityName,
				To:          fieldDesc.GetMessageType().GetName(),
				Type:        OneToOne,
				Cardinality: "1:1",
				Metadata: map[string]interface{}{
					"field_name":  fieldDesc.GetName(),
					"proto_field": true,
				},
			}
			relations = append(relations, relation)
		}
	}

	// Add JSON tags for serialization
	property.Tags["json"] = fieldDesc.GetName()

	// Add protobuf tags
	property.Tags["protobuf"] = fmt.Sprintf("varint,%d,opt,name=%s", fieldDesc.GetNumber(), fieldDesc.GetName())

	// Add validation if field is required
	if property.Required {
		property.Tags["validate"] = "required"
	}

	return property, relations, nil
}

// mapProtoType maps Protocol Buffer types to Go types
func (p *ProtoParser) mapProtoType(fieldDesc *desc.FieldDescriptor, entityName string) (string, []Relation, error) {
	relations := make([]Relation, 0)

	switch fieldDesc.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
		return "float64", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		return "float32", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_INT64:
		return "int64", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_UINT64:
		return "uint64", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_INT32:
		return "int32", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_FIXED64:
		return "uint64", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_FIXED32:
		return "uint32", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return "bool", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return "string", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return "[]byte", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_UINT32:
		return "uint32", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_SFIXED32:
		return "int32", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		return "int64", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_SINT32:
		return "int32", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_SINT64:
		return "int64", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
		// Handle message types
		if fieldDesc.GetMessageType() != nil {
			msgName := fieldDesc.GetMessageType().GetName()
			// Skip request/response messages in type mapping
			if p.isRequestResponseMessage(msgName) {
				return "interface{}", relations, nil
			}
			return msgName, relations, nil
		}
		return "interface{}", relations, nil
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		// Handle enum types - map to string for simplicity
		if fieldDesc.GetEnumType() != nil {
			return "string", relations, nil
		}
		return "string", relations, nil
	default:
		return "interface{}", relations, nil
	}
}

// convertFieldName converts proto field names to Go field names (snake_case to PascalCase)
func (p *ProtoParser) convertFieldName(protoName string) string {
	parts := strings.Split(protoName, "_")
	result := ""
	for _, part := range parts {
		if len(part) > 0 {
			result += strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	return result
}

// isRequestResponseMessage checks if a message is a request/response DTO
func (p *ProtoParser) isRequestResponseMessage(messageName string) bool {
	lowerName := strings.ToLower(messageName)
	return strings.HasSuffix(lowerName, "request") ||
		strings.HasSuffix(lowerName, "response") ||
		strings.HasSuffix(lowerName, "req") ||
		strings.HasSuffix(lowerName, "resp")
}

// extractProjectName extracts project name from Protocol Buffer file or package
func (p *ProtoParser) extractProjectName(fileDesc *desc.FileDescriptor, filePath string) string {
	if fileDesc == nil {
		// Fallback to filename without extension
		filename := filepath.Base(filePath)
		ext := filepath.Ext(filename)
		return strings.TrimSuffix(filename, ext)
	}

	// Try to extract from go_package option
	if goPackage := p.extractGoPackage(fileDesc); goPackage != "" {
		parts := strings.Split(goPackage, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Try to use package name
	if pkg := fileDesc.GetPackage(); pkg != "" {
		return strings.ReplaceAll(pkg, ".", "-")
	}

	// Fallback to filename without extension
	filename := filepath.Base(filePath)
	ext := filepath.Ext(filename)
	return strings.TrimSuffix(filename, ext)
}

// extractGoPackage extracts the go_package option from the proto file
func (p *ProtoParser) extractGoPackage(fileDesc *desc.FileDescriptor) string {
	if fileDesc == nil {
		return ""
	}

	options := fileDesc.GetOptions()
	if options == nil {
		return ""
	}

	// Try to get go_package option using reflection
	if opts, ok := options.(*descriptorpb.FileOptions); ok && opts != nil && opts.GoPackage != nil {
		return *opts.GoPackage
	}
	return ""
}

// extractServiceInfo extracts service information for metadata
func (p *ProtoParser) extractServiceInfo(fileDesc *desc.FileDescriptor) []map[string]interface{} {
	services := make([]map[string]interface{}, 0)

	for _, serviceDesc := range fileDesc.GetServices() {
		methods := make([]map[string]interface{}, 0)
		for _, methodDesc := range serviceDesc.GetMethods() {
			methods = append(methods, map[string]interface{}{
				"name":        methodDesc.GetName(),
				"input_type":  methodDesc.GetInputType().GetName(),
				"output_type": methodDesc.GetOutputType().GetName(),
			})
		}

		services = append(services, map[string]interface{}{
			"name":    serviceDesc.GetName(),
			"methods": methods,
		})
	}

	return services
}
