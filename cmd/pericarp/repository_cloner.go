package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// RepositoryCloner handles Git repository operations
type RepositoryCloner struct {
	logger CliLogger
}

// NewRepositoryCloner creates a new repository cloner instance
func NewRepositoryCloner(logger CliLogger) *RepositoryCloner {
	return &RepositoryCloner{
		logger: logger,
	}
}

// CloneRepository clones a Git repository to the specified destination (Requirement 4.1, 4.2)
func (r *RepositoryCloner) CloneRepository(repoURL, destination string) error {
	r.logger.Infof("Cloning repository: %s", repoURL)

	// Check if destination already exists
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		return NewCliError(FileSystemError,
			fmt.Sprintf("destination directory already exists: %s", destination),
			nil)
	}

	// Clone the repository with proper error handling (Requirement 4.1)
	cmd := exec.Command("git", "clone", repoURL, destination)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Clean up partial clone on failure
		if _, statErr := os.Stat(destination); statErr == nil {
			os.RemoveAll(destination)
		}
		return NewCliError(NetworkError,
			fmt.Sprintf("failed to clone repository %s", repoURL),
			err)
	}

	r.logger.Info("Repository cloned successfully")
	return nil
}

// ValidateRepository checks if the cloned repository is valid (Requirement 4.2)
func (r *RepositoryCloner) ValidateRepository(path string) error {
	// Check if it's a valid Git repository
	if _, err := os.Stat(filepath.Join(path, ".git")); os.IsNotExist(err) {
		return NewCliError(ValidationError,
			"cloned directory is not a valid Git repository",
			err)
	}

	r.logger.Debugf("Repository validation successful: %s", path)
	return nil
}

// PreserveExistingFiles ensures existing files are not overwritten (Requirement 4.4, 4.5)
func (r *RepositoryCloner) PreserveExistingFiles(destination string, newFiles []*GeneratedFile) ([]*GeneratedFile, error) {
	var safeFiles []*GeneratedFile
	var skippedFiles []string

	for _, file := range newFiles {
		fullPath := filepath.Join(destination, file.Path)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			// File doesn't exist, safe to create
			safeFiles = append(safeFiles, file)
		} else {
			// File exists, skip it to preserve existing content
			skippedFiles = append(skippedFiles, file.Path)
			r.logger.Warnf("Skipping existing file: %s", file.Path)
		}
	}

	if len(skippedFiles) > 0 {
		r.logger.Infof("Preserved %d existing files", len(skippedFiles))
		r.logger.Debug("Skipped files: " + fmt.Sprintf("%v", skippedFiles))
	}

	return safeFiles, nil
}

// CheckGitAvailability verifies that Git is available on the system
func (r *RepositoryCloner) CheckGitAvailability() error {
	cmd := exec.Command("git", "--version")
	if err := cmd.Run(); err != nil {
		return NewCliError(ValidationError,
			"Git is not available on this system. Please install Git to use repository cloning features",
			err)
	}
	return nil
}
