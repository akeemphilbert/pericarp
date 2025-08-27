package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepositoryCloner_CloneRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "pericarp-clone-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	destination := filepath.Join(tempDir, "test-repo")

	// Test cloning a real repository (using a small public repo)
	repoURL := "https://github.com/octocat/Hello-World.git"

	err = cloner.CloneRepository(repoURL, destination)
	assert.NoError(t, err)

	// Verify the repository was cloned
	assert.DirExists(t, destination)
	assert.DirExists(t, filepath.Join(destination, ".git"))

	// Verify repository validation passes
	err = cloner.ValidateRepository(destination)
	assert.NoError(t, err)
}

func TestRepositoryCloner_CloneRepository_InvalidURL(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir, err := os.MkdirTemp("", "pericarp-clone-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	destination := filepath.Join(tempDir, "test-repo")

	// Test cloning with invalid URL
	repoURL := "https://github.com/nonexistent/invalid-repo.git"

	err = cloner.CloneRepository(repoURL, destination)
	assert.Error(t, err)

	// Verify destination was cleaned up on failure
	assert.NoDirExists(t, destination)
}

func TestRepositoryCloner_CloneRepository_ExistingDestination(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir, err := os.MkdirTemp("", "pericarp-clone-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	destination := filepath.Join(tempDir, "existing-dir")

	// Create the destination directory first
	err = os.MkdirAll(destination, 0755)
	require.NoError(t, err)

	repoURL := "https://github.com/octocat/Hello-World.git"

	err = cloner.CloneRepository(repoURL, destination)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "destination directory already exists")
}

func TestRepositoryCloner_ValidateRepository(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir, err := os.MkdirTemp("", "pericarp-validate-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Test with non-git directory
	nonGitDir := filepath.Join(tempDir, "not-git")
	err = os.MkdirAll(nonGitDir, 0755)
	require.NoError(t, err)

	err = cloner.ValidateRepository(nonGitDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid Git repository")

	// Test with valid git directory
	gitDir := filepath.Join(tempDir, "git-repo")
	err = os.MkdirAll(filepath.Join(gitDir, ".git"), 0755)
	require.NoError(t, err)

	err = cloner.ValidateRepository(gitDir)
	assert.NoError(t, err)
}

func TestRepositoryCloner_PreserveExistingFiles(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir, err := os.MkdirTemp("", "pericarp-preserve-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create some existing files
	existingFile := filepath.Join(tempDir, "existing.go")
	err = os.WriteFile(existingFile, []byte("existing content"), 0644)
	require.NoError(t, err)

	// Create test files to be generated
	newFiles := []*GeneratedFile{
		{
			Path:    "existing.go",
			Content: "new content",
		},
		{
			Path:    "new.go",
			Content: "new file content",
		},
	}

	safeFiles, err := cloner.PreserveExistingFiles(tempDir, newFiles)
	assert.NoError(t, err)

	// Should only return the new file, not the existing one
	assert.Len(t, safeFiles, 1)
	assert.Equal(t, "new.go", safeFiles[0].Path)
}

func TestRepositoryCloner_CheckGitAvailability(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	err := cloner.CheckGitAvailability()
	// This should pass on most systems with Git installed
	// If Git is not available, it should return a proper error
	if err != nil {
		assert.Contains(t, err.Error(), "Git is not available")
	}
}

func TestProjectCreator_CreateProject_WithRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	tempDir, err := os.MkdirTemp("", "pericarp-project-repo-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	projectName := "test-service"
	destination := filepath.Join(tempDir, projectName)
	repoURL := "https://github.com/octocat/Hello-World.git"

	// Test creating project with repository cloning
	err = creator.CreateProject(projectName, repoURL, destination, false)
	assert.NoError(t, err)

	// Verify repository was cloned
	assert.DirExists(t, destination)
	assert.DirExists(t, filepath.Join(destination, ".git"))

	// Verify Pericarp files were added
	assert.FileExists(t, filepath.Join(destination, "go.mod"))
	assert.FileExists(t, filepath.Join(destination, "Makefile"))
	assert.FileExists(t, filepath.Join(destination, "README.md"))
	assert.DirExists(t, filepath.Join(destination, "cmd", projectName))
}

func TestProjectCreator_CreateProject_WithRepository_DryRun(t *testing.T) {
	logger := NewTestLogger()
	creator := NewProjectCreator(logger)

	tempDir, err := os.MkdirTemp("", "pericarp-project-repo-dryrun-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	projectName := "test-service"
	destination := filepath.Join(tempDir, projectName)
	repoURL := "https://github.com/octocat/Hello-World.git"

	// Test dry run with repository cloning
	err = creator.CreateProject(projectName, repoURL, destination, true)
	assert.NoError(t, err)

	// Verify nothing was actually created in dry run mode
	assert.NoDirExists(t, destination)
}

func TestProjectCreator_CreateProject_WithRepository_FilePreservation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := NewTestLogger()

	tempDir, err := os.MkdirTemp("", "pericarp-project-preserve-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// First, create a repository with some existing files
	repoDir := filepath.Join(tempDir, "existing-repo")
	err = os.MkdirAll(filepath.Join(repoDir, ".git"), 0755)
	require.NoError(t, err)

	// Create an existing README.md
	existingReadme := filepath.Join(repoDir, "README.md")
	existingContent := "# Existing Project\nThis is an existing project."
	err = os.WriteFile(existingReadme, []byte(existingContent), 0644)
	require.NoError(t, err)

	// Now use the project creator to add Pericarp capabilities
	projectName := "test-service"

	// Create a mock domain model and generate files
	domainModel := &DomainModel{
		ProjectName: projectName,
		Entities:    []Entity{},
		Relations:   []Relation{},
		Metadata: map[string]interface{}{
			"generated_by": "pericarp-cli",
			"version":      "test",
		},
	}

	factory, err := NewPericarpComponentFactory(logger)
	require.NoError(t, err)

	// Generate project files
	files, err := factory.GenerateProjectFiles(domainModel)
	require.NoError(t, err)

	// Use the cloner to preserve existing files
	cloner := NewRepositoryCloner(logger)
	safeFiles, err := cloner.PreserveExistingFiles(repoDir, files)
	require.NoError(t, err)

	// Verify that README.md was preserved (not in safe files)
	readmePreserved := true
	for _, file := range safeFiles {
		if file.Path == "README.md" {
			readmePreserved = false
			break
		}
	}
	assert.True(t, readmePreserved, "README.md should have been preserved")

	// Verify existing content is still there
	content, err := os.ReadFile(existingReadme)
	require.NoError(t, err)
	assert.Equal(t, existingContent, string(content))
}
