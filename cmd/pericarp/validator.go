package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// DefaultValidator implements Validator interface
type DefaultValidator struct {
	logger CliLogger
}

// NewValidator creates a new validator instance
func NewValidator() Validator {
	return &DefaultValidator{
		logger: NewCliLogger(),
	}
}

// ValidateProjectName ensures project names follow Go module conventions (Requirement 10.1, 10.2)
func (v *DefaultValidator) ValidateProjectName(name string) error {
	if name == "" {
		return NewCliError(ValidationError, "project name cannot be empty", nil)
	}

	// Check for valid Go module name format
	if !regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$|^[a-z]$`).MatchString(name) {
		return NewCliError(ValidationError,
			"project name must start with lowercase letter and contain only lowercase letters, numbers, and hyphens",
			nil)
	}

	// Check for reserved names
	reservedNames := []string{"con", "prn", "aux", "nul", "com1", "com2", "com3", "com4", "com5", "com6", "com7", "com8", "com9", "lpt1", "lpt2", "lpt3", "lpt4", "lpt5", "lpt6", "lpt7", "lpt8", "lpt9"}
	for _, reserved := range reservedNames {
		if name == reserved {
			return NewCliError(ValidationError,
				fmt.Sprintf("project name '%s' is reserved and cannot be used", name),
				nil)
		}
	}

	return nil
}

// ValidateInputFile checks if input files exist and are readable (Requirement 10.2, 10.3)
func (v *DefaultValidator) ValidateInputFile(filePath string) error {
	if filePath == "" {
		return NewCliError(ValidationError, "input file path cannot be empty", nil)
	}

	// Check if file exists
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return NewCliError(FileSystemError,
			fmt.Sprintf("input file does not exist: %s", filePath),
			err)
	}
	if err != nil {
		return NewCliError(FileSystemError,
			fmt.Sprintf("cannot access input file: %s", filePath),
			err)
	}

	// Check if it's a file (not a directory)
	if info.IsDir() {
		return NewCliError(ValidationError,
			fmt.Sprintf("input path is a directory, not a file: %s", filePath),
			nil)
	}

	// Check if file is readable
	file, err := os.Open(filePath)
	if err != nil {
		return NewCliError(FileSystemError,
			fmt.Sprintf("cannot read input file: %s", filePath),
			err)
	}
	file.Close()

	return nil
}

// ValidateDestination ensures destination directory is writable (Requirement 10.2, 10.3)
func (v *DefaultValidator) ValidateDestination(path string) error {
	if path == "" {
		return nil // Will use default
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return NewCliError(FileSystemError,
			fmt.Sprintf("cannot resolve destination path: %s", path),
			err)
	}

	// Check if destination already exists
	if info, err := os.Stat(absPath); err == nil {
		if !info.IsDir() {
			return NewCliError(ValidationError,
				fmt.Sprintf("destination exists but is not a directory: %s", absPath),
				nil)
		}
		// Directory exists, check if it's writable
		testFile := filepath.Join(absPath, ".pericarp-write-test")
		if file, err := os.Create(testFile); err != nil {
			return NewCliError(FileSystemError,
				fmt.Sprintf("destination directory is not writable: %s", absPath),
				err)
		} else {
			file.Close()
			os.Remove(testFile)
		}
		return nil
	}

	// Destination doesn't exist, check if parent directory exists and is writable
	parent := filepath.Dir(absPath)
	if info, err := os.Stat(parent); os.IsNotExist(err) {
		return NewCliError(FileSystemError,
			fmt.Sprintf("destination parent directory does not exist: %s", parent),
			err)
	} else if err != nil {
		return NewCliError(FileSystemError,
			fmt.Sprintf("cannot access destination parent directory: %s", parent),
			err)
	} else if !info.IsDir() {
		return NewCliError(ValidationError,
			fmt.Sprintf("destination parent is not a directory: %s", parent),
			nil)
	}

	// Check if parent is writable
	testFile := filepath.Join(parent, ".pericarp-write-test")
	if file, err := os.Create(testFile); err != nil {
		return NewCliError(FileSystemError,
			fmt.Sprintf("destination parent directory is not writable: %s", parent),
			err)
	} else {
		file.Close()
		os.Remove(testFile)
	}

	return nil
}
