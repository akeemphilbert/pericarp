package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultExecutor implements Executor interface
type DefaultExecutor struct {
	logger CliLogger
}

// NewExecutor creates a new executor instance
func NewExecutor(logger CliLogger) Executor {
	return &DefaultExecutor{
		logger: logger,
	}
}

// Execute creates files or shows preview in dry-run mode (Requirement 9.1, 9.6)
func (e *DefaultExecutor) Execute(ctx context.Context, files []*GeneratedFile, destination string, dryRun bool) error {
	if len(files) == 0 {
		e.logger.Info("No files to generate")
		return nil
	}

	if dryRun {
		return e.executeDryRun(files, destination)
	}

	return e.executeReal(ctx, files, destination)
}

// executeDryRun shows what would be generated without creating files (Requirement 9.1, 9.6)
func (e *DefaultExecutor) executeDryRun(files []*GeneratedFile, destination string) error {
	e.logger.Info("DRY RUN MODE - No files will be created")
	e.logger.Info("Target destination", "path", destination)
	e.logger.Info("Would generate files", "count", len(files))

	for i, file := range files {
		fullPath := filepath.Join(destination, file.Path)
		e.logger.Infof("  %d. %s", i+1, fullPath)

		if e.logger.IsVerbose() {
			e.logger.Debug("File content preview", "file", file.Path)
			preview := e.truncateContent(file.Content, 500)
			for _, line := range strings.Split(preview, "\n") {
				e.logger.Debugf("    %s", line)
			}
			if len(file.Content) > 500 {
				e.logger.Debug("Content truncated for preview")
			}
		}
	}

	return nil
}

// executeReal creates the actual files (Requirement 2.4)
func (e *DefaultExecutor) executeReal(ctx context.Context, files []*GeneratedFile, destination string) error {
	e.logger.Info("Generating files", "count", len(files), "destination", destination)

	// Create destination directory if it doesn't exist
	if err := os.MkdirAll(destination, 0755); err != nil {
		return NewCliError(FileSystemError,
			fmt.Sprintf("failed to create destination directory: %s", destination),
			err)
	}

	// Generate each file
	for i, file := range files {
		select {
		case <-ctx.Done():
			return NewCliError(GenerationError, "file generation was cancelled", ctx.Err())
		default:
		}

		fullPath := filepath.Join(destination, file.Path)

		if e.logger.IsVerbose() {
			e.logger.Debug("Creating file", "progress", fmt.Sprintf("%d/%d", i+1, len(files)), "path", fullPath)
		}

		// Create directory for file if it doesn't exist
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return NewCliError(FileSystemError,
				fmt.Sprintf("failed to create directory for file %s", fullPath),
				err)
		}

		// Write file content
		if err := os.WriteFile(fullPath, []byte(file.Content), 0644); err != nil {
			return NewCliError(FileSystemError,
				fmt.Sprintf("failed to write file %s", fullPath),
				err)
		}

		e.logger.Infof("✓ Created: %s", file.Path)
	}

	e.logger.Info("Successfully generated files", "count", len(files))
	return nil
}

// truncateContent truncates content to specified length for preview
func (e *DefaultExecutor) truncateContent(content string, maxLength int) string {
	if len(content) <= maxLength {
		return content
	}
	return content[:maxLength]
}
