package main

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"text/template"
	"unicode"
)

//go:embed templates/*
var templateFS embed.FS

// TemplateEngine handles Go template loading and execution for code generation
// Implements template helper functions and manages template directory structure (Requirement 8.1, 8.2, 8.3)
type TemplateEngine struct {
	templates map[string]*template.Template
	funcMap   template.FuncMap
	logger    CliLogger
}

// NewTemplateEngine creates a new template engine with helper functions
func NewTemplateEngine(logger CliLogger) (*TemplateEngine, error) {
	engine := &TemplateEngine{
		templates: make(map[string]*template.Template),
		logger:    logger,
	}

	// Initialize template helper functions
	engine.funcMap = template.FuncMap{
		"lower":          strings.ToLower,
		"upper":          strings.ToUpper,
		"title":          strings.Title,
		"camelCase":      engine.toCamelCase,
		"snakeCase":      engine.toSnakeCase,
		"kebabCase":      engine.toKebabCase,
		"pascalCase":     engine.toPascalCase,
		"plural":         engine.toPlural,
		"singular":       engine.toSingular,
		"slice":          engine.sliceString,
		"join":           strings.Join,
		"contains":       strings.Contains,
		"hasPrefix":      strings.HasPrefix,
		"hasSuffix":      strings.HasSuffix,
		"replace":        strings.ReplaceAll,
		"trim":           strings.TrimSpace,
		"indent":         engine.indent,
		"zeroValue":      engine.getZeroValue,
		"goType":         engine.toGoType,
		"jsonTag":        engine.toJSONTag,
		"validationTag":  engine.toValidationTag,
		"isRequired":     engine.isRequired,
		"filterRequired": engine.filterRequired,
		"filterOptional": engine.filterOptional,
		"last":           engine.isLast,
		"ternary":        engine.ternary,
		"gt":             engine.gt,
		"len":            engine.length,
		"default":        engine.defaultValue,
	}

	// Load all templates from embedded filesystem
	if err := engine.loadTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	return engine, nil
}

// loadTemplates loads all template files from the embedded filesystem
func (te *TemplateEngine) loadTemplates() error {
	return fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".tmpl") {
			return nil
		}

		// Read template content
		content, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read template %s: %w", path, err)
		}

		// Create template name from path (remove templates/ prefix and .tmpl suffix)
		templateName := strings.TrimPrefix(path, "templates/")
		templateName = strings.TrimSuffix(templateName, ".tmpl")

		// Parse template with helper functions
		tmpl, err := template.New(templateName).Funcs(te.funcMap).Parse(string(content))
		if err != nil {
			return fmt.Errorf("failed to parse template %s: %w", templateName, err)
		}

		te.templates[templateName] = tmpl
		te.logger.Debug(fmt.Sprintf("Loaded template: %s", templateName))

		return nil
	})
}

// Execute executes a template with the given data
func (te *TemplateEngine) Execute(templateName string, data interface{}) (string, error) {
	tmpl, exists := te.templates[templateName]
	if !exists {
		return "", fmt.Errorf("template not found: %s", templateName)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	return buf.String(), nil
}

// HasTemplate checks if a template exists
func (te *TemplateEngine) HasTemplate(templateName string) bool {
	_, exists := te.templates[templateName]
	return exists
}

// ListTemplates returns all available template names
func (te *TemplateEngine) ListTemplates() []string {
	names := make([]string, 0, len(te.templates))
	for name := range te.templates {
		names = append(names, name)
	}
	return names
}

// Template helper functions

// toCamelCase converts a string to camelCase
func (te *TemplateEngine) toCamelCase(s string) string {
	if s == "" {
		return s
	}

	words := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	if len(words) == 0 {
		// Handle PascalCase to camelCase conversion
		if len(s) > 0 {
			return strings.ToLower(s[:1]) + s[1:]
		}
		return s
	}

	// If there's only one word and it's already lowercase, check if it contains separators
	if len(words) == 1 && !strings.ContainsAny(s, "_-") {
		// Already camelCase or single word
		return strings.ToLower(s[:1]) + s[1:]
	}

	result := strings.ToLower(words[0])
	for i := 1; i < len(words); i++ {
		result += strings.Title(strings.ToLower(words[i]))
	}

	return result
}

// toPascalCase converts a string to PascalCase
func (te *TemplateEngine) toPascalCase(s string) string {
	if s == "" {
		return s
	}

	words := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	var result strings.Builder
	for _, word := range words {
		result.WriteString(strings.Title(strings.ToLower(word)))
	}

	return result.String()
}

// toSnakeCase converts a string to snake_case
func (te *TemplateEngine) toSnakeCase(s string) string {
	if s == "" {
		return s
	}

	// Handle common abbreviations
	if s == "ID" {
		return "id"
	}

	var result strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			result.WriteRune('_')
		}
		result.WriteRune(unicode.ToLower(r))
	}

	return result.String()
}

// toKebabCase converts a string to kebab-case
func (te *TemplateEngine) toKebabCase(s string) string {
	if s == "" {
		return s
	}

	var result strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			result.WriteRune('-')
		}
		result.WriteRune(unicode.ToLower(r))
	}

	return result.String()
}

// toPlural converts a string to plural form (simple implementation)
func (te *TemplateEngine) toPlural(s string) string {
	if s == "" {
		return s
	}

	s = strings.ToLower(s)
	if strings.HasSuffix(s, "y") {
		return s[:len(s)-1] + "ies"
	}
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "sh") || strings.HasSuffix(s, "ch") {
		return s + "es"
	}
	return s + "s"
}

// toSingular converts a string to singular form (simple implementation)
func (te *TemplateEngine) toSingular(s string) string {
	if s == "" {
		return s
	}

	s = strings.ToLower(s)
	if strings.HasSuffix(s, "ies") {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "es") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "s") && len(s) > 1 {
		return s[:len(s)-1]
	}
	return s
}

// sliceString slices a string from start to end
func (te *TemplateEngine) sliceString(s string, start, end int) string {
	if start < 0 || start >= len(s) {
		return ""
	}
	if end < 0 || end > len(s) {
		end = len(s)
	}
	if start >= end {
		return ""
	}
	return s[start:end]
}

// indent adds indentation to each line of a string
func (te *TemplateEngine) indent(s string, spaces int) string {
	if s == "" {
		return s
	}

	indentation := strings.Repeat(" ", spaces)
	lines := strings.Split(s, "\n")

	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[i] = indentation + line
		}
	}

	return strings.Join(lines, "\n")
}

// getZeroValue returns the zero value for a Go type
func (te *TemplateEngine) getZeroValue(goType string) string {
	switch goType {
	case "string":
		return `""`
	case "int", "int8", "int16", "int32", "int64":
		return "0"
	case "uint", "uint8", "uint16", "uint32", "uint64":
		return "0"
	case "float32", "float64":
		return "0.0"
	case "bool":
		return "false"
	case "time.Time":
		return "time.Time{}"
	case "ksuid.KSUID":
		return "ksuid.KSUID{}"
	default:
		if strings.HasPrefix(goType, "*") {
			return "nil"
		}
		if strings.HasPrefix(goType, "[]") {
			return "nil"
		}
		if strings.HasPrefix(goType, "map[") {
			return "nil"
		}
		return goType + "{}"
	}
}

// toGoType converts a generic type to Go type
func (te *TemplateEngine) toGoType(genericType string) string {
	switch strings.ToLower(genericType) {
	case "string", "text":
		return "string"
	case "int", "integer":
		return "int"
	case "int64", "long":
		return "int64"
	case "float", "float64", "double":
		return "float64"
	case "float32":
		return "float32"
	case "bool", "boolean":
		return "bool"
	case "uuid", "guid":
		return "ksuid.KSUID"
	case "time", "datetime", "timestamp":
		return "time.Time"
	case "date":
		return "time.Time"
	default:
		return genericType
	}
}

// toJSONTag creates a JSON struct tag
func (te *TemplateEngine) toJSONTag(fieldName string, required bool) string {
	jsonName := te.toSnakeCase(fieldName)
	if required {
		return fmt.Sprintf(`json:"%s"`, jsonName)
	}
	return fmt.Sprintf(`json:"%s,omitempty"`, jsonName)
}

// toValidationTag creates a validation struct tag
func (te *TemplateEngine) toValidationTag(property Property) string {
	var validations []string

	if property.Required {
		validations = append(validations, "required")
	}

	if property.Validation != "" {
		validations = append(validations, property.Validation)
	}

	if len(validations) == 0 {
		return ""
	}

	return fmt.Sprintf(`validate:"%s"`, strings.Join(validations, ","))
}

// isRequired checks if a property is required
func (te *TemplateEngine) isRequired(property Property) bool {
	return property.Required
}

// filterRequired filters properties to only required ones
func (te *TemplateEngine) filterRequired(properties []Property) []Property {
	var required []Property
	for _, prop := range properties {
		if prop.Required {
			required = append(required, prop)
		}
	}
	return required
}

// filterOptional filters properties to only optional ones
func (te *TemplateEngine) filterOptional(properties []Property) []Property {
	var optional []Property
	for _, prop := range properties {
		if !prop.Required {
			optional = append(optional, prop)
		}
	}
	return optional
}

// isLast checks if the current index is the last in a slice
func (te *TemplateEngine) isLast(index int, slice interface{}) bool {
	switch s := slice.(type) {
	case []Entity:
		return index == len(s)-1
	case []Property:
		return index == len(s)-1
	case []string:
		return index == len(s)-1
	default:
		return false
	}
}

// ternary implements ternary operator (condition ? trueValue : falseValue)
func (te *TemplateEngine) ternary(condition bool, trueValue, falseValue interface{}) interface{} {
	if condition {
		return trueValue
	}
	return falseValue
}

// gt checks if a > b
func (te *TemplateEngine) gt(a, b interface{}) bool {
	switch av := a.(type) {
	case int:
		if bv, ok := b.(int); ok {
			return av > bv
		}
	case int64:
		if bv, ok := b.(int64); ok {
			return av > bv
		}
	}
	return false
}

// length returns the length of a slice, array, map, or string
func (te *TemplateEngine) length(v interface{}) int {
	switch val := v.(type) {
	case []Entity:
		return len(val)
	case []Property:
		return len(val)
	case []string:
		return len(val)
	case string:
		return len(val)
	case map[string]interface{}:
		return len(val)
	default:
		return 0
	}
}

// defaultValue returns the default value if the input is empty
func (te *TemplateEngine) defaultValue(value, defaultVal interface{}) interface{} {
	switch v := value.(type) {
	case string:
		if v == "" {
			return defaultVal
		}
		return v
	case nil:
		return defaultVal
	default:
		return value
	}
}
