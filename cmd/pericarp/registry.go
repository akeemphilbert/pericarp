package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

// DefaultParserRegistry implements ParserRegistry interface
type DefaultParserRegistry struct {
	parsers map[string]DomainParser
}

// NewParserRegistry creates a new parser registry
func NewParserRegistry() ParserRegistry {
	return &DefaultParserRegistry{
		parsers: make(map[string]DomainParser),
	}
}

// RegisterParser adds a new parser to the registry (Requirement 7.1, 7.2)
func (r *DefaultParserRegistry) RegisterParser(parser DomainParser) error {
	if parser == nil {
		return NewCliError(ArgumentError, "parser cannot be nil", nil)
	}

	for _, ext := range parser.SupportedExtensions() {
		if ext == "" {
			return NewCliError(ArgumentError, "parser extension cannot be empty", nil)
		}
		r.parsers[strings.ToLower(ext)] = parser
	}

	return nil
}

// GetParser returns the appropriate parser for a file extension (Requirement 7.3)
func (r *DefaultParserRegistry) GetParser(filePath string) (DomainParser, error) {
	if filePath == "" {
		return nil, NewCliError(ArgumentError, "file path cannot be empty", nil)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if parser, exists := r.parsers[ext]; exists {
		return parser, nil
	}

	return nil, NewCliError(ParseError,
		fmt.Sprintf("no parser found for file extension: %s", ext),
		nil)
}

// ListFormats returns all supported formats (Requirement 7.5)
func (r *DefaultParserRegistry) ListFormats() []string {
	formatMap := make(map[string]bool)
	var formats []string

	for _, parser := range r.parsers {
		formatName := parser.FormatName()
		if !formatMap[formatName] {
			formats = append(formats, formatName)
			formatMap[formatName] = true
		}
	}

	return formats
}

// DetectFormat attempts to automatically detect the input format (Requirement 7.4)
func (r *DefaultParserRegistry) DetectFormat(filePath string) (string, error) {
	if filePath == "" {
		return "", NewCliError(ArgumentError, "file path cannot be empty", nil)
	}

	parser, err := r.GetParser(filePath)
	if err != nil {
		return "", err
	}

	// Validate that the file is actually parseable by this parser
	if err := parser.Validate(filePath); err != nil {
		return "", NewCliError(ParseError,
			fmt.Sprintf("file %s is not a valid %s file", filePath, parser.FormatName()),
			err)
	}

	return parser.FormatName(), nil
}
