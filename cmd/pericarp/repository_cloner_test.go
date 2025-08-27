package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRepositoryCloner(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	assert.NotNil(t, cloner)
	assert.Equal(t, logger, cloner.logger)
}

func TestRepositoryCloner_CheckGitAvailability_Unit(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	err := cloner.CheckGitAvailability()

	// This test depends on Git being available on the system
	// In most development environments, Git should be available
	if err != nil {
		// If Git is not available, verify the error is correct
		cliErr, ok := err.(*CliError)
		require.True(t, ok)
		assert.Equal(t, ValidationError, cliErr.Type)
		assert.Contains(t, cliErr.Message, "Git is not available")
	} else {
		// Git is available, test should pass
		assert.NoError(t, err)
	}
}

func TestRepositoryCloner_ValidateRepository_Unit(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tests := []struct {
		name        string
		setupRepo   func(string) error
		expectError bool
		errorType   ErrorType
	}{
		{
			name: "valid git repository",
			setupRepo: func(path string) error {
				return os.MkdirAll(filepath.Join(path, ".git"), 0755)
			},
			expectError: false,
		},
		{
			name: "directory without .git",
			setupRepo: func(path string) error {
				return os.MkdirAll(path, 0755)
			},
			expectError: true,
			errorType:   ValidationError,
		},
		{
			name: "non-existent directory",
			setupRepo: func(path string) error {
				// Don't create anything
				return nil
			},
			expectError: true,
			errorType:   ValidationError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			repoPath := filepath.Join(tempDir, "test-repo")

			err := tt.setupRepo(repoPath)
			require.NoError(t, err)

			err = cloner.ValidateRepository(repoPath)

			if tt.expectError {
				assert.Error(t, err)
				cliErr, ok := err.(*CliError)
				require.True(t, ok)
				assert.Equal(t, tt.errorType, cliErr.Type)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRepositoryCloner_PreserveExistingFiles_Unit(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir := t.TempDir()

	// Create some existing files
	existingFile1 := filepath.Join(tempDir, "existing1.go")
	existingFile2 := filepath.Join(tempDir, "subdir", "existing2.go")

	err := os.MkdirAll(filepath.Dir(existingFile2), 0755)
	require.NoError(t, err)

	err = os.WriteFile(existingFile1, []byte("existing content 1"), 0644)
	require.NoError(t, err)

	err = os.WriteFile(existingFile2, []byte("existing content 2"), 0644)
	require.NoError(t, err)

	// Create a list of files to generate (some conflict, some don't)
	newFiles := []*GeneratedFile{
		{
			Path:    "existing1.go", // This conflicts
			Content: "new content 1",
		},
		{
			Path:    "subdir/existing2.go", // This conflicts
			Content: "new content 2",
		},
		{
			Path:    "new1.go", // This doesn't conflict
			Content: "new content 3",
		},
		{
			Path:    "newdir/new2.go", // This doesn't conflict
			Content: "new content 4",
		},
	}

	safeFiles, err := cloner.PreserveExistingFiles(tempDir, newFiles)

	assert.NoError(t, err)
	assert.Len(t, safeFiles, 2) // Only the non-conflicting files

	// Verify the safe files are the non-conflicting ones
	safeFilePaths := make(map[string]bool)
	for _, file := range safeFiles {
		safeFilePaths[file.Path] = true
	}

	assert.True(t, safeFilePaths["new1.go"])
	assert.True(t, safeFilePaths["newdir/new2.go"])
	assert.False(t, safeFilePaths["existing1.go"])
	assert.False(t, safeFilePaths["subdir/existing2.go"])

	// Verify existing files were not modified
	content1, err := os.ReadFile(existingFile1)
	require.NoError(t, err)
	assert.Equal(t, "existing content 1", string(content1))

	content2, err := os.ReadFile(existingFile2)
	require.NoError(t, err)
	assert.Equal(t, "existing content 2", string(content2))
}

func TestRepositoryCloner_PreserveExistingFiles_NoConflicts(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir := t.TempDir()

	// Create files that don't conflict
	newFiles := []*GeneratedFile{
		{
			Path:    "new1.go",
			Content: "new content 1",
		},
		{
			Path:    "subdir/new2.go",
			Content: "new content 2",
		},
	}

	safeFiles, err := cloner.PreserveExistingFiles(tempDir, newFiles)

	assert.NoError(t, err)
	assert.Len(t, safeFiles, 2) // All files are safe
	assert.Equal(t, newFiles, safeFiles)
}

func TestRepositoryCloner_PreserveExistingFiles_AllConflicts(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir := t.TempDir()

	// Create existing files
	existingFile := filepath.Join(tempDir, "conflict.go")
	err := os.WriteFile(existingFile, []byte("existing"), 0644)
	require.NoError(t, err)

	// Create files that all conflict
	newFiles := []*GeneratedFile{
		{
			Path:    "conflict.go",
			Content: "new content",
		},
	}

	safeFiles, err := cloner.PreserveExistingFiles(tempDir, newFiles)

	assert.NoError(t, err)
	assert.Len(t, safeFiles, 0) // No files are safe
}

func TestRepositoryCloner_PreserveExistingFiles_EmptyFileList(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir := t.TempDir()

	safeFiles, err := cloner.PreserveExistingFiles(tempDir, []*GeneratedFile{})

	assert.NoError(t, err)
	assert.Len(t, safeFiles, 0)
}

// TestRepositoryCloner_CloneRepository_DestinationExists tests that cloning fails when destination exists
func TestRepositoryCloner_CloneRepository_DestinationExists(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir := t.TempDir()
	destination := filepath.Join(tempDir, "existing")

	// Create the destination directory
	err := os.MkdirAll(destination, 0755)
	require.NoError(t, err)

	err = cloner.CloneRepository("https://github.com/example/repo.git", destination)

	assert.Error(t, err)
	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, FileSystemError, cliErr.Type)
	assert.Contains(t, cliErr.Message, "destination directory already exists")
}

// TestRepositoryCloner_CloneRepository_InvalidURL_Unit tests cloning with invalid URL
func TestRepositoryCloner_CloneRepository_InvalidURL_Unit(t *testing.T) {
	// Skip this test if Git is not available
	if !isGitAvailable() {
		t.Skip("Git is not available, skipping clone test")
	}

	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir := t.TempDir()
	destination := filepath.Join(tempDir, "invalid-clone")

	// Use an invalid URL that will cause git clone to fail
	err := cloner.CloneRepository("invalid-url", destination)

	assert.Error(t, err)
	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, NetworkError, cliErr.Type)
	assert.Contains(t, cliErr.Message, "failed to clone repository")

	// Verify destination was cleaned up on failure
	assert.NoDirExists(t, destination)
}

// TestRepositoryCloner_CloneRepository_NetworkError simulates network error
func TestRepositoryCloner_CloneRepository_NetworkError(t *testing.T) {
	// Skip this test if Git is not available
	if !isGitAvailable() {
		t.Skip("Git is not available, skipping clone test")
	}

	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir := t.TempDir()
	destination := filepath.Join(tempDir, "network-error-clone")

	// Use a URL that will timeout or fail
	err := cloner.CloneRepository("https://nonexistent-domain-12345.com/repo.git", destination)

	assert.Error(t, err)
	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, NetworkError, cliErr.Type)

	// Verify destination was cleaned up on failure
	assert.NoDirExists(t, destination)
}

// TestRepositoryCloner_Integration tests the complete workflow
func TestRepositoryCloner_Integration_MockGitRepo(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir := t.TempDir()

	// Create a mock local Git repository to clone from
	sourceRepo := filepath.Join(tempDir, "source")
	err := os.MkdirAll(sourceRepo, 0755)
	require.NoError(t, err)

	// Initialize a bare Git repository if Git is available
	if isGitAvailable() {
		cmd := exec.Command("git", "init", "--bare", sourceRepo)
		err = cmd.Run()
		require.NoError(t, err)

		// Create a temporary working directory to add some content
		workDir := filepath.Join(tempDir, "work")
		err = os.MkdirAll(workDir, 0755)
		require.NoError(t, err)

		// Clone the bare repo to working directory
		cmd = exec.Command("git", "clone", sourceRepo, workDir)
		err = cmd.Run()
		require.NoError(t, err)

		// Add some content
		testFile := filepath.Join(workDir, "README.md")
		err = os.WriteFile(testFile, []byte("# Test Repository"), 0644)
		require.NoError(t, err)

		// Configure git user for the test
		cmd = exec.Command("git", "config", "user.email", "test@example.com")
		cmd.Dir = workDir
		err = cmd.Run()
		require.NoError(t, err)

		cmd = exec.Command("git", "config", "user.name", "Test User")
		cmd.Dir = workDir
		err = cmd.Run()
		require.NoError(t, err)

		// Add and commit
		cmd = exec.Command("git", "add", ".")
		cmd.Dir = workDir
		err = cmd.Run()
		require.NoError(t, err)

		cmd = exec.Command("git", "commit", "-m", "Initial commit")
		cmd.Dir = workDir
		err = cmd.Run()
		require.NoError(t, err)

		// Push to bare repo
		cmd = exec.Command("git", "push", "origin", "main")
		cmd.Dir = workDir
		err = cmd.Run()
		if err != nil {
			// Try with master branch if main doesn't work
			cmd = exec.Command("git", "push", "origin", "master")
			cmd.Dir = workDir
			err = cmd.Run()
		}
		require.NoError(t, err)

		// Now test cloning from the local bare repository
		destination := filepath.Join(tempDir, "cloned")

		err = cloner.CloneRepository(sourceRepo, destination)
		assert.NoError(t, err)

		// Validate the cloned repository
		err = cloner.ValidateRepository(destination)
		assert.NoError(t, err)

		// Verify content was cloned
		clonedFile := filepath.Join(destination, "README.md")
		assert.FileExists(t, clonedFile)

		content, err := os.ReadFile(clonedFile)
		require.NoError(t, err)
		assert.Equal(t, "# Test Repository", string(content))
	} else {
		t.Skip("Git is not available, skipping integration test")
	}
}

// TestRepositoryCloner_PreserveExistingFiles_Integration tests file preservation in realistic scenario
func TestRepositoryCloner_PreserveExistingFiles_Integration(t *testing.T) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir := t.TempDir()

	// Simulate an existing repository with some files
	existingFiles := map[string]string{
		"README.md":                   "# Existing Project\n\nThis is an existing project.",
		"go.mod":                      "module existing-project\n\ngo 1.21",
		"main.go":                     "package main\n\nfunc main() {\n\tprintln(\"existing\")\n}",
		"internal/domain/existing.go": "package domain\n\ntype Existing struct {\n\tID string\n}",
	}

	// Create existing files
	for filePath, content := range existingFiles {
		fullPath := filepath.Join(tempDir, filePath)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	// Simulate new files that would be generated by Pericarp
	newFiles := []*GeneratedFile{
		{
			Path:    "README.md", // Conflicts with existing
			Content: "# Generated Project\n\nThis is a generated project.",
		},
		{
			Path:    "go.mod", // Conflicts with existing
			Content: "module generated-project\n\ngo 1.21",
		},
		{
			Path:    "Makefile", // New file, no conflict
			Content: "deps:\n\tgo mod download\n",
		},
		{
			Path:    "internal/domain/user.go", // New file, no conflict
			Content: "package domain\n\ntype User struct {\n\tID string\n\tName string\n}",
		},
		{
			Path:    "internal/application/user_commands.go", // New file, no conflict
			Content: "package application\n\ntype CreateUserCommand struct {\n\tName string\n}",
		},
	}

	safeFiles, err := cloner.PreserveExistingFiles(tempDir, newFiles)

	assert.NoError(t, err)
	assert.Len(t, safeFiles, 3) // Only non-conflicting files

	// Verify safe files are the expected ones
	safeFilePaths := make(map[string]bool)
	for _, file := range safeFiles {
		safeFilePaths[file.Path] = true
	}

	assert.True(t, safeFilePaths["Makefile"])
	assert.True(t, safeFilePaths["internal/domain/user.go"])
	assert.True(t, safeFilePaths["internal/application/user_commands.go"])
	assert.False(t, safeFilePaths["README.md"])
	assert.False(t, safeFilePaths["go.mod"])

	// Verify existing files were not modified
	for filePath, expectedContent := range existingFiles {
		fullPath := filepath.Join(tempDir, filePath)
		content, err := os.ReadFile(fullPath)
		require.NoError(t, err)
		assert.Equal(t, expectedContent, string(content), "Existing file %s should not be modified", filePath)
	}
}

// Helper function to check if Git is available
func isGitAvailable() bool {
	cmd := exec.Command("git", "--version")
	return cmd.Run() == nil
}

// Benchmark tests
func BenchmarkRepositoryCloner_PreserveExistingFiles(b *testing.B) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir := b.TempDir()

	// Create some existing files
	for i := 0; i < 10; i++ {
		filePath := filepath.Join(tempDir, fmt.Sprintf("existing%d.go", i))
		err := os.WriteFile(filePath, []byte("existing content"), 0644)
		if err != nil {
			b.Fatal(err)
		}
	}

	// Create a large list of files to check
	var newFiles []*GeneratedFile
	for i := 0; i < 100; i++ {
		newFiles = append(newFiles, &GeneratedFile{
			Path:    fmt.Sprintf("file%d.go", i),
			Content: "new content",
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := cloner.PreserveExistingFiles(tempDir, newFiles)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRepositoryCloner_ValidateRepository(b *testing.B) {
	logger := NewTestLogger()
	cloner := NewRepositoryCloner(logger)

	tempDir := b.TempDir()
	repoPath := filepath.Join(tempDir, "test-repo")

	// Create a mock .git directory
	err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0755)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := cloner.ValidateRepository(repoPath)
		if err != nil {
			b.Fatal(err)
		}
	}
}
