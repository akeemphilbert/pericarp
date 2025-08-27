package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// DryRunExecutor implements preview mode without file creation (Requirement 9.1, 9.6)
type DryRunExecutor struct {
	logger CliLogger
}

// NewDryRunExecutor creates a new dry-run executor instance
func NewDryRunExecutor(logger CliLogger) *DryRunExecutor {
	return &DryRunExecutor{
		logger: logger,
	}
}

// Execute shows what would be generated without creating files (Requirement 9.1, 9.6)
func (d *DryRunExecutor) Execute(ctx context.Context, files []*GeneratedFile, destination string, dryRun bool) error {
	if !dryRun {
		return fmt.Errorf("DryRunExecutor should only be used in dry-run mode")
	}

	return d.previewGeneration(files, destination)
}

// previewGeneration displays file content preview and destination paths (Requirement 9.1, 9.6)
func (d *DryRunExecutor) previewGeneration(files []*GeneratedFile, destination string) error {
	d.logger.Info("=== DRY RUN MODE - No files will be created ===")
	d.logger.Info("Target destination", "path", destination)
	d.logger.Info("Files to be generated", "count", len(files))
	d.logger.Info("")

	if len(files) == 0 {
		d.logger.Info("No files to generate")
		return nil
	}

	// Group files by directory for better organization
	filesByDir := d.groupFilesByDirectory(files, destination)

	// Display files organized by directory
	for dir, dirFiles := range filesByDir {
		d.logger.Infof("Directory: %s", dir)
		for _, file := range dirFiles {
			d.logger.Infof("  ✓ %s", file.Path)

			if d.logger.IsVerbose() {
				d.displayFilePreview(file)
			}
		}
		d.logger.Info("")
	}

	// Summary
	d.logger.Info("=== Dry Run Summary ===")
	d.logger.Info("Total files", "count", len(files))
	d.logger.Info("Total directories", "count", len(filesByDir))
	d.logger.Info("Destination", "path", destination)

	if !d.logger.IsVerbose() {
		d.logger.Info("Use --verbose flag to see file content previews")
	}

	return nil
}

// displayFilePreview shows file content preview when verbose mode is enabled
func (d *DryRunExecutor) displayFilePreview(file *GeneratedFile) {
	d.logger.Debug("File content preview", "file", file.Path)

	// Show file metadata if available
	if len(file.Metadata) > 0 {
		d.logger.Debug("File metadata:")
		for key, value := range file.Metadata {
			d.logger.Debugf("  %s: %v", key, value)
		}
	}

	// Show content preview
	preview := d.truncateContent(file.Content, 500)
	lines := strings.Split(preview, "\n")

	d.logger.Debug("Content preview (first 500 characters):")
	d.logger.Debug("┌" + strings.Repeat("─", 60) + "┐")

	for i, line := range lines {
		if i >= 20 { // Limit to first 20 lines for readability
			d.logger.Debug("│ ... (content truncated)")
			break
		}
		// Truncate long lines
		if len(line) > 58 {
			line = line[:55] + "..."
		}
		d.logger.Debugf("│ %s", line)
	}

	d.logger.Debug("└" + strings.Repeat("─", 60) + "┘")

	if len(file.Content) > 500 {
		d.logger.Debug("Content truncated for preview", "total_size", len(file.Content))
	}

	d.logger.Debug("") // Empty line for separation
}

// groupFilesByDirectory organizes files by their directory for better display
func (d *DryRunExecutor) groupFilesByDirectory(files []*GeneratedFile, destination string) map[string][]*GeneratedFile {
	filesByDir := make(map[string][]*GeneratedFile)

	for _, file := range files {
		dir := filepath.Dir(file.Path)
		if dir == "." {
			dir = "root"
		}
		filesByDir[dir] = append(filesByDir[dir], file)
	}

	return filesByDir
}

// truncateContent truncates content to specified length for preview
func (d *DryRunExecutor) truncateContent(content string, maxLength int) string {
	if len(content) <= maxLength {
		return content
	}
	return content[:maxLength]
}

// PreviewProjectStructure shows the directory structure that would be created
func (d *DryRunExecutor) PreviewProjectStructure(model *DomainModel, destination string) error {
	d.logger.Info("=== Project Structure Preview ===")
	d.logger.Info("Project name", "name", model.ProjectName)
	d.logger.Info("Destination", "path", destination)
	d.logger.Info("")

	// Show expected directory structure
	d.logger.Info("Directory structure to be created:")
	d.logger.Info(".")
	d.logger.Info("├── go.mod")
	d.logger.Info("├── go.sum")
	d.logger.Info("├── Makefile")
	d.logger.Info("├── README.md")
	d.logger.Info("├── config.yaml.example")
	d.logger.Info("├── cmd/")
	d.logger.Infof("│   └── %s/", model.ProjectName)
	d.logger.Info("│       └── main.go")
	d.logger.Info("├── internal/")
	d.logger.Info("│   ├── application/")
	d.logger.Info("│   │   ├── commands.go")
	d.logger.Info("│   │   ├── queries.go")
	d.logger.Info("│   │   └── handlers.go")
	d.logger.Info("│   ├── domain/")

	for _, entity := range model.Entities {
		d.logger.Infof("│   │   ├── %s.go", strings.ToLower(entity.Name))
		d.logger.Infof("│   │   ├── %s_events.go", strings.ToLower(entity.Name))
		d.logger.Infof("│   │   └── %s_test.go", strings.ToLower(entity.Name))
	}

	d.logger.Info("│   └── infrastructure/")
	d.logger.Info("│       └── repositories.go")
	d.logger.Info("├── pkg/")
	d.logger.Info("└── test/")
	d.logger.Info("    ├── fixtures/")
	d.logger.Info("    ├── integration/")
	d.logger.Info("    └── mocks/")
	d.logger.Info("")

	if d.logger.IsVerbose() {
		d.logger.Debug("Entities to be generated", "count", len(model.Entities))
		for _, entity := range model.Entities {
			d.logger.Debugf("  - %s (%d properties)", entity.Name, len(entity.Properties))
		}
	}

	return nil
}
