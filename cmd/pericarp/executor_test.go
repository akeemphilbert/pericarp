package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewExecutor(t *testing.T) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)

	assert.NotNil(t, executor)
	assert.IsType(t, &DefaultExecutor{}, executor)
}

func TestDefaultExecutor_Execute_EmptyFiles(t *testing.T) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)
	ctx := context.Background()

	err := executor.Execute(ctx, []*GeneratedFile{}, "test-dest", false)
	assert.NoError(t, err)
}

func TestDefaultExecutor_Execute_DryRun(t *testing.T) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)
	ctx := context.Background()

	files := []*GeneratedFile{
		{
			Path:    "test.go",
			Content: "package main\n\nfunc main() {}\n",
		},
		{
			Path:    "internal/domain/user.go",
			Content: "package domain\n\ntype User struct {\n\tID string\n}\n",
		},
	}

	tempDir := t.TempDir()
	err := executor.Execute(ctx, files, tempDir, true)

	assert.NoError(t, err)

	// Verify no files were actually created
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "No files should be created in dry-run mode")
}

func TestDefaultExecutor_Execute_RealFiles(t *testing.T) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)
	ctx := context.Background()

	files := []*GeneratedFile{
		{
			Path:    "main.go",
			Content: "package main\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}\n",
		},
		{
			Path:    "internal/domain/user.go",
			Content: "package domain\n\ntype User struct {\n\tID string `json:\"id\"`\n\tName string `json:\"name\"`\n}\n",
		},
		{
			Path:    "pkg/utils/helper.go",
			Content: "package utils\n\nfunc Helper() string {\n\treturn \"helper\"\n}\n",
		},
	}

	tempDir := t.TempDir()
	err := executor.Execute(ctx, files, tempDir, false)

	assert.NoError(t, err)

	// Verify files were created with correct content
	for _, file := range files {
		fullPath := filepath.Join(tempDir, file.Path)
		assert.FileExists(t, fullPath)

		content, err := os.ReadFile(fullPath)
		require.NoError(t, err)
		assert.Equal(t, file.Content, string(content))
	}

	// Verify directory structure was created
	assert.DirExists(t, filepath.Join(tempDir, "internal", "domain"))
	assert.DirExists(t, filepath.Join(tempDir, "pkg", "utils"))
}

func TestDefaultExecutor_Execute_ContextCancellation(t *testing.T) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	files := []*GeneratedFile{
		{
			Path:    "test.go",
			Content: "package main",
		},
	}

	tempDir := t.TempDir()
	err := executor.Execute(ctx, files, tempDir, false)

	assert.Error(t, err)
	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, GenerationError, cliErr.Type)
	assert.Contains(t, cliErr.Message, "file generation was cancelled")
}

func TestDefaultExecutor_Execute_ContextTimeout(t *testing.T) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)

	// Create a context with very short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to timeout
	time.Sleep(1 * time.Millisecond)

	files := []*GeneratedFile{
		{
			Path:    "test.go",
			Content: "package main",
		},
	}

	tempDir := t.TempDir()
	err := executor.Execute(ctx, files, tempDir, false)

	assert.Error(t, err)
	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, GenerationError, cliErr.Type)
	assert.Contains(t, cliErr.Message, "file generation was cancelled")
}

func TestDefaultExecutor_Execute_DirectoryCreationError(t *testing.T) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)
	ctx := context.Background()

	files := []*GeneratedFile{
		{
			Path:    "test.go",
			Content: "package main",
		},
	}

	// Try to create files in a non-existent parent directory with restricted permissions
	tempDir := t.TempDir()
	restrictedDir := filepath.Join(tempDir, "restricted")
	err := os.Mkdir(restrictedDir, 0000) // No permissions
	require.NoError(t, err)
	defer os.Chmod(restrictedDir, 0755) // Cleanup

	invalidDest := filepath.Join(restrictedDir, "subdir")
	err = executor.Execute(ctx, files, invalidDest, false)

	assert.Error(t, err)
	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, FileSystemError, cliErr.Type)
	assert.Contains(t, cliErr.Message, "failed to create destination directory")
}

func TestDefaultExecutor_Execute_FileWriteError(t *testing.T) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)
	ctx := context.Background()

	files := []*GeneratedFile{
		{
			Path:    "restricted/test.go",
			Content: "package main",
		},
	}

	tempDir := t.TempDir()

	// Create a directory with restricted permissions
	restrictedDir := filepath.Join(tempDir, "restricted")
	err := os.Mkdir(restrictedDir, 0000) // No write permissions
	require.NoError(t, err)
	defer os.Chmod(restrictedDir, 0755) // Cleanup

	err = executor.Execute(ctx, files, tempDir, false)

	assert.Error(t, err)
	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, FileSystemError, cliErr.Type)
	assert.Contains(t, cliErr.Message, "failed to write file")
}

func TestDefaultExecutor_Execute_VerboseLogging(t *testing.T) {
	logger := NewTestLogger()
	logger.SetVerbose(true)
	executor := NewExecutor(logger)
	ctx := context.Background()

	files := []*GeneratedFile{
		{
			Path:    "test.go",
			Content: "package main\n\nfunc main() {}\n",
		},
	}

	tempDir := t.TempDir()
	err := executor.Execute(ctx, files, tempDir, false)

	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(tempDir, "test.go"))
}

func TestDefaultExecutor_executeDryRun(t *testing.T) {
	logger := NewTestLogger()
	executor := &DefaultExecutor{logger: logger}

	files := []*GeneratedFile{
		{
			Path:    "main.go",
			Content: "package main\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}\n",
		},
		{
			Path:    "internal/domain/user.go",
			Content: "package domain\n\ntype User struct {\n\tID string\n}\n",
		},
	}

	err := executor.executeDryRun(files, "/tmp/test-project")
	assert.NoError(t, err)
}

func TestDefaultExecutor_executeDryRun_VerboseMode(t *testing.T) {
	logger := NewTestLogger()
	logger.SetVerbose(true)
	executor := &DefaultExecutor{logger: logger}

	files := []*GeneratedFile{
		{
			Path:    "test.go",
			Content: strings.Repeat("a", 1000), // Long content to test truncation
		},
	}

	err := executor.executeDryRun(files, "/tmp/test-project")
	assert.NoError(t, err)
}

func TestDefaultExecutor_executeReal(t *testing.T) {
	logger := NewTestLogger()
	executor := &DefaultExecutor{logger: logger}
	ctx := context.Background()

	files := []*GeneratedFile{
		{
			Path:    "simple.go",
			Content: "package main",
		},
	}

	tempDir := t.TempDir()
	err := executor.executeReal(ctx, files, tempDir)

	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(tempDir, "simple.go"))
}

func TestDefaultExecutor_truncateContent(t *testing.T) {
	logger := NewTestLogger()
	executor := &DefaultExecutor{logger: logger}

	tests := []struct {
		name      string
		content   string
		maxLength int
		expected  string
	}{
		{
			name:      "content shorter than max",
			content:   "short content",
			maxLength: 100,
			expected:  "short content",
		},
		{
			name:      "content longer than max",
			content:   "this is a very long content that should be truncated",
			maxLength: 20,
			expected:  "this is a very long ",
		},
		{
			name:      "empty content",
			content:   "",
			maxLength: 10,
			expected:  "",
		},
		{
			name:      "exact length",
			content:   "exact",
			maxLength: 5,
			expected:  "exact",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.truncateContent(tt.content, tt.maxLength)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultExecutor_Execute_LargeNumberOfFiles(t *testing.T) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)
	ctx := context.Background()

	// Generate a large number of files
	var files []*GeneratedFile
	for i := 0; i < 100; i++ {
		files = append(files, &GeneratedFile{
			Path:    fmt.Sprintf("file_%d.go", i),
			Content: fmt.Sprintf("package main\n\n// File %d\nfunc main() {}\n", i),
		})
	}

	tempDir := t.TempDir()
	err := executor.Execute(ctx, files, tempDir, false)

	assert.NoError(t, err)

	// Verify all files were created
	for i := 0; i < 100; i++ {
		filePath := filepath.Join(tempDir, fmt.Sprintf("file_%d.go", i))
		assert.FileExists(t, filePath)
	}
}

func TestDefaultExecutor_Execute_NestedDirectories(t *testing.T) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)
	ctx := context.Background()

	files := []*GeneratedFile{
		{
			Path:    "level1/level2/level3/deep.go",
			Content: "package deep",
		},
		{
			Path:    "another/path/file.go",
			Content: "package another",
		},
	}

	tempDir := t.TempDir()
	err := executor.Execute(ctx, files, tempDir, false)

	assert.NoError(t, err)

	// Verify nested directories were created
	assert.DirExists(t, filepath.Join(tempDir, "level1", "level2", "level3"))
	assert.DirExists(t, filepath.Join(tempDir, "another", "path"))
	assert.FileExists(t, filepath.Join(tempDir, "level1", "level2", "level3", "deep.go"))
	assert.FileExists(t, filepath.Join(tempDir, "another", "path", "file.go"))
}

func TestDefaultExecutor_Execute_FilePermissions(t *testing.T) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)
	ctx := context.Background()

	files := []*GeneratedFile{
		{
			Path:    "test.go",
			Content: "package main",
		},
	}

	tempDir := t.TempDir()
	err := executor.Execute(ctx, files, tempDir, false)

	assert.NoError(t, err)

	// Check file permissions
	filePath := filepath.Join(tempDir, "test.go")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
}

func TestDefaultExecutor_Execute_DirectoryPermissions(t *testing.T) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)
	ctx := context.Background()

	files := []*GeneratedFile{
		{
			Path:    "subdir/test.go",
			Content: "package main",
		},
	}

	tempDir := t.TempDir()
	err := executor.Execute(ctx, files, tempDir, false)

	assert.NoError(t, err)

	// Check directory permissions
	dirPath := filepath.Join(tempDir, "subdir")
	info, err := os.Stat(dirPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

// Benchmark tests
func BenchmarkDefaultExecutor_Execute_DryRun(b *testing.B) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)
	ctx := context.Background()

	files := []*GeneratedFile{
		{Path: "test1.go", Content: "package main\n\nfunc main() {}"},
		{Path: "test2.go", Content: "package main\n\nfunc test() {}"},
		{Path: "test3.go", Content: "package main\n\nfunc helper() {}"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := executor.Execute(ctx, files, "/tmp/benchmark", true)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDefaultExecutor_Execute_RealFiles(b *testing.B) {
	logger := NewTestLogger()
	executor := NewExecutor(logger)
	ctx := context.Background()

	files := []*GeneratedFile{
		{Path: "test.go", Content: "package main\n\nfunc main() {}"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tempDir := b.TempDir()
		err := executor.Execute(ctx, files, tempDir, false)
		if err != nil {
			b.Fatal(err)
		}
	}
}
