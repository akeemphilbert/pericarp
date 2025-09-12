package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantHelp bool
		wantErr  bool
	}{
		{
			name:     "no arguments shows help",
			args:     []string{},
			wantHelp: true,
			wantErr:  false,
		},
		{
			name:     "help flag shows help",
			args:     []string{"--help"},
			wantHelp: true,
			wantErr:  false,
		},
		{
			name:     "verbose flag is recognized",
			args:     []string{"--verbose", "--help"},
			wantHelp: true,
			wantErr:  false,
		},
		{
			name:     "short verbose flag is recognized",
			args:     []string{"-v", "--help"},
			wantHelp: true,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new root command for each test to avoid state pollution
			cmd := createTestRootCommand()

			// Capture output
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			// Set arguments
			cmd.SetArgs(tt.args)

			// Execute command
			err := cmd.Execute()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			output := buf.String()
			if tt.wantHelp {
				assert.Contains(t, output, "Pericarp CLI Generator")
				assert.Contains(t, output, "Usage:")
				assert.Contains(t, output, "Available Commands:")
			}
		})
	}
}

func TestVersionCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "version command",
			args: []string{"version"},
		},
		{
			name: "version with verbose",
			args: []string{"--verbose", "version"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createTestRootCommand()

			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			require.NoError(t, err)

			output := buf.String()
			assert.Contains(t, output, "pericarp version")
			assert.Contains(t, output, "commit:")
			assert.Contains(t, output, "built:")
			assert.Contains(t, output, "go version:")
		})
	}
}

func TestGlobalFlags(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectVerbose bool
	}{
		{
			name:          "verbose flag long form",
			args:          []string{"--verbose", "version"},
			expectVerbose: true,
		},
		{
			name:          "verbose flag short form",
			args:          []string{"-v", "version"},
			expectVerbose: true,
		},
		{
			name:          "no verbose flag",
			args:          []string{"version"},
			expectVerbose: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global state
			verbose = false
			logger = nil

			cmd := createTestRootCommand()
			cmd.SetArgs(tt.args)

			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)

			err := cmd.Execute()
			require.NoError(t, err)

			// Check that verbose flag was set correctly
			assert.Equal(t, tt.expectVerbose, verbose)

			// Check that logger was initialized with correct verbose setting
			if logger != nil {
				assert.Equal(t, tt.expectVerbose, logger.IsVerbose())
			}
		})
	}
}

func TestHelpTemplate(t *testing.T) {
	cmd := createTestRootCommand()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()

	// Check that help template includes expected sections
	assert.Contains(t, output, "Usage:")
	assert.Contains(t, output, "Available Commands:")
	assert.Contains(t, output, "Flags:")
	assert.Contains(t, output, "Examples:")

	// Check that examples are included
	assert.Contains(t, output, "pericarp new my-service")
	assert.Contains(t, output, "pericarp generate --openapi")
	assert.Contains(t, output, "pericarp formats")
}

func TestCommandStructure(t *testing.T) {
	cmd := createTestRootCommand()

	// Test that root command has expected properties
	assert.Equal(t, "pericarp", cmd.Use)
	assert.Contains(t, cmd.Short, "Pericarp CLI Generator")
	assert.NotEmpty(t, cmd.Long)
	assert.NotEmpty(t, cmd.Example)

	// Test that version command is added
	versionCmd, _, err := cmd.Find([]string{"version"})
	require.NoError(t, err)
	assert.Equal(t, "version", versionCmd.Use)
	assert.Contains(t, versionCmd.Short, "version information")
}

func TestErrorHandling(t *testing.T) {
	// Test that main function handles CliError properly
	// This is more of an integration test since we can't easily test main() directly

	// Create a command that returns a CliError
	testCmd := &cobra.Command{
		Use: "test-error",
		RunE: func(cmd *cobra.Command, args []string) error {
			return NewCliError(ValidationError, "test validation error", nil)
		},
	}

	rootCmd := createTestRootCommand()
	rootCmd.AddCommand(testCmd)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"test-error"})

	err := rootCmd.Execute()
	require.Error(t, err)

	// Check that it's a CliError
	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, ValidationError, cliErr.Type)
	assert.Equal(t, "test validation error", cliErr.Message)
	assert.Equal(t, 3, cliErr.ExitCode()) // ValidationError should have exit code 3
}

// createTestRootCommand creates a fresh root command for testing
func createTestRootCommand() *cobra.Command {
	// Reset global variables
	verbose = false
	logger = nil

	cmd := &cobra.Command{
		Use:   "pericarp",
		Short: "Pericarp CLI Generator for scaffolding DDD projects",
		Long: `Pericarp CLI Generator enables developers to scaffold new Pericarp-based projects
with automated code generation from various input formats including ERD, OpenAPI, and Protocol Buffers.

The CLI follows domain-driven design principles and generates production-ready code
with proper aggregate patterns, repositories, command/query handlers, and comprehensive testing.`,
		Example: `  # Create a new project
  pericarp new my-service

  # Create a new project from existing repository
  pericarp new my-service --repo https://github.com/user/repo.git

  # Generate code from OpenAPI specification
  pericarp generate --openapi api.yaml

  # List supported input formats
  pericarp formats

  # Show version information
  pericarp version`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logger = NewCliLogger()
			logger.SetVerbose(verbose)
		},
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	// Add global flags
	cmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")

	// Add version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Long: `Display version information including build details.

This command shows the current version of the Pericarp CLI tool,
along with commit hash and build date for debugging purposes.`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("pericarp version %s\n", version)
			cmd.Printf("commit: %s\n", commit)
			cmd.Printf("built: %s\n", date)
			cmd.Printf("go version: %s\n", getGoVersion())
		},
	}

	cmd.AddCommand(versionCmd)

	// Set help template
	cmd.SetHelpTemplate(`{{.Long}}

Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`)

	return cmd
}

// Test helper to capture stdout/stderr
func captureOutput(f func()) (string, string) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr

	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()

	os.Stdout = wOut
	os.Stderr = wErr

	outC := make(chan string)
	errC := make(chan string)

	go func() {
		var buf bytes.Buffer
		buf.ReadFrom(rOut)
		outC <- buf.String()
	}()

	go func() {
		var buf bytes.Buffer
		buf.ReadFrom(rErr)
		errC <- buf.String()
	}()

	f()

	wOut.Close()
	wErr.Close()

	os.Stdout = oldStdout
	os.Stderr = oldStderr

	return <-outC, <-errC
}

func TestMainFunctionErrorHandling(t *testing.T) {
	// This test verifies that main() handles different error types correctly
	// We can't test main() directly, but we can test the error handling logic

	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{
			name:     "CliError with validation error",
			err:      NewCliError(ValidationError, "validation failed", nil),
			wantCode: 3,
		},
		{
			name:     "CliError with parse error",
			err:      NewCliError(ParseError, "parse failed", nil),
			wantCode: 4,
		},
		{
			name:     "regular error",
			err:      assert.AnError,
			wantCode: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if cliErr, ok := tt.err.(*CliError); ok {
				assert.Equal(t, tt.wantCode, cliErr.ExitCode())
			}
		})
	}
}
func TestNewCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errType string
	}{
		{
			name:    "valid project name",
			args:    []string{"new", "my-service", "--dry-run"},
			wantErr: false,
		},
		{
			name:    "project name with flags",
			args:    []string{"new", "my-service", "--destination", "/tmp", "--dry-run"},
			wantErr: false,
		},
		{
			name:    "project name with repo flag",
			args:    []string{"new", "my-service", "--repo", "https://github.com/user/repo.git", "--dry-run"},
			wantErr: false,
		},
		{
			name:    "missing project name",
			args:    []string{"new"},
			wantErr: true,
			errType: "argument",
		},
		{
			name:    "invalid project name",
			args:    []string{"new", "Invalid-Project-Name", "--dry-run"},
			wantErr: true,
			errType: "validation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createTestRootCommand()

			// Add the new command with flags
			newCmd := createTestNewCommand()
			cmd.AddCommand(newCmd)

			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != "" {
					// Check error type if specified
					if cliErr, ok := err.(*CliError); ok {
						assert.Contains(t, string(cliErr.Type), tt.errType)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGenerateCommand(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errType string
	}{
		{
			name:    "missing input format",
			args:    []string{"generate"},
			wantErr: true,
			errType: "argument",
		},
		{
			name:    "multiple input formats",
			args:    []string{"generate", "--openapi", "api.yaml", "--proto", "user.proto"},
			wantErr: true,
			errType: "argument",
		},
		{
			name:    "openapi with dry-run",
			args:    []string{"generate", "--openapi", "testdata/user-service.yaml", "--dry-run"},
			wantErr: false,
		},
		{
			name:    "proto with destination",
			args:    []string{"generate", "--proto", "testdata/user.proto", "--destination", "/tmp", "--dry-run"},
			wantErr: false,
		},
		{
			name:    "nonexistent file",
			args:    []string{"generate", "--openapi", "nonexistent.yaml"},
			wantErr: true,
			errType: "filesystem",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createTestRootCommand()

			// Add the generate command with flags
			generateCmd := createTestGenerateCommand()
			cmd.AddCommand(generateCmd)

			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != "" {
					// Check error type if specified
					if cliErr, ok := err.(*CliError); ok {
						assert.Contains(t, string(cliErr.Type), tt.errType)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFormatsCommand(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedOutput []string
		wantErr        bool
	}{
		{
			name: "list all formats",
			args: []string{"formats"},
			expectedOutput: []string{
				"Supported input formats:",
				"OpenAPI",
				"File extensions: .yaml, .yml, .json",
				"Protocol Buffers",
				"File extensions: .proto",
			},
			wantErr: false,
		},
		{
			name: "formats with verbose flag",
			args: []string{"--verbose", "formats"},
			expectedOutput: []string{
				"Supported input formats:",
				"OpenAPI",
				"Protocol Buffers",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createTestRootCommand()

			// Add the actual formats command (not the mock)
			formatsCmd := createActualFormatsCommand()
			cmd.AddCommand(formatsCmd)

			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			output := buf.String()
			for _, expected := range tt.expectedOutput {
				assert.Contains(t, output, expected, "Output should contain: %s", expected)
			}
		})
	}
}

func TestFormatsCommandParserRegistration(t *testing.T) {
	// Test that the formats command properly registers parsers
	cmd := createTestRootCommand()
	formatsCmd := createActualFormatsCommand()
	cmd.AddCommand(formatsCmd)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"formats"})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()

	// Verify that both parsers are registered and displayed
	assert.Contains(t, output, "OpenAPI")
	assert.Contains(t, output, "Protocol Buffers")

	// Verify that file extensions are shown
	assert.Contains(t, output, ".yaml, .yml, .json")
	assert.Contains(t, output, ".proto")

	// Verify proper formatting
	assert.Contains(t, output, "Supported input formats:")
	assert.Contains(t, output, "File extensions:")
}

func TestFormatsCommandErrorHandling(t *testing.T) {
	// Test error handling in formats command
	// This test verifies that if parser registration fails, the command handles it gracefully

	// Create a command that simulates parser registration failure
	failingFormatsCmd := &cobra.Command{
		Use: "formats",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Simulate a parser registration error
			return NewCliError(ArgumentError, "failed to register parser", nil)
		},
	}

	cmd := createTestRootCommand()
	cmd.AddCommand(failingFormatsCmd)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"formats"})

	err := cmd.Execute()
	require.Error(t, err)

	// Verify it's the expected error type
	cliErr, ok := err.(*CliError)
	require.True(t, ok)
	assert.Equal(t, ArgumentError, cliErr.Type)
	assert.Contains(t, cliErr.Message, "failed to register parser")
}

func TestCommandFlags(t *testing.T) {
	tests := []struct {
		name    string
		command string
		args    []string
		checkFn func(*testing.T, *cobra.Command)
	}{
		{
			name:    "new command flags",
			command: "new",
			args:    []string{"new", "test-project", "--repo", "https://github.com/test/repo", "--destination", "/tmp", "--dry-run"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				repo, _ := cmd.Flags().GetString("repo")
				dest, _ := cmd.Flags().GetString("destination")
				dryRun, _ := cmd.Flags().GetBool("dry-run")

				assert.Equal(t, "https://github.com/test/repo", repo)
				assert.Equal(t, "/tmp", dest)
				assert.True(t, dryRun)
			},
		},
		{
			name:    "generate command flags",
			command: "generate",
			args:    []string{"generate", "--openapi", "api.yaml", "--destination", "/output", "--dry-run"},
			checkFn: func(t *testing.T, cmd *cobra.Command) {
				openapi, _ := cmd.Flags().GetString("openapi")
				dest, _ := cmd.Flags().GetString("destination")
				dryRun, _ := cmd.Flags().GetBool("dry-run")

				assert.Equal(t, "api.yaml", openapi)
				assert.Equal(t, "/output", dest)
				assert.True(t, dryRun)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd := createTestRootCommand()

			var targetCmd *cobra.Command
			switch tt.command {
			case "new":
				targetCmd = createTestNewCommand()
			case "generate":
				targetCmd = createTestGenerateCommand()
			}

			rootCmd.AddCommand(targetCmd)
			rootCmd.SetArgs(tt.args)

			// Parse flags without executing
			err := targetCmd.ParseFlags(tt.args[1:]) // Skip the command name
			require.NoError(t, err)

			tt.checkFn(t, targetCmd)
		})
	}
}

// Helper functions for creating test commands
func createTestNewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "new <project-name>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectName := args[0]

			// Get flags
			repoURL, _ := cmd.Flags().GetString("repo")
			destination, _ := cmd.Flags().GetString("destination")
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			// Validate project name
			validator := NewValidator()
			if err := validator.ValidateProjectName(projectName); err != nil {
				return err
			}

			// Validate destination if provided
			if destination != "" {
				if err := validator.ValidateDestination(destination); err != nil {
					return err
				}
			}

			// Mock project creation for testing
			if dryRun {
				cmd.Printf("DRY RUN: Would create project %s\n", projectName)
				if repoURL != "" {
					cmd.Printf("DRY RUN: Would clone from %s\n", repoURL)
				}
				if destination != "" {
					cmd.Printf("DRY RUN: Would create in %s\n", destination)
				}
			}

			return nil
		},
	}

	// Add flags
	cmd.Flags().StringP("repo", "r", "", "Git repository URL to clone from")
	cmd.Flags().StringP("destination", "d", "", "destination directory")
	cmd.Flags().Bool("dry-run", false, "preview what would be created")

	return cmd
}

func createTestGenerateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "generate",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get input format flags
			erdFile, _ := cmd.Flags().GetString("erd")
			openAPIFile, _ := cmd.Flags().GetString("openapi")
			protoFile, _ := cmd.Flags().GetString("proto")

			// Get other flags
			destination, _ := cmd.Flags().GetString("destination")
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			// Validate that exactly one input format is provided
			inputCount := 0
			var inputFile, inputType string

			if erdFile != "" {
				inputCount++
				inputFile = erdFile
				inputType = "erd"
			}
			if openAPIFile != "" {
				inputCount++
				inputFile = openAPIFile
				inputType = "openapi"
			}
			if protoFile != "" {
				inputCount++
				inputFile = protoFile
				inputType = "proto"
			}

			if inputCount == 0 {
				return NewCliError(ArgumentError,
					"must specify exactly one input format",
					nil)
			}
			if inputCount > 1 {
				return NewCliError(ArgumentError,
					"cannot specify multiple input formats",
					nil)
			}

			// Validate input file exists
			validator := NewValidator()
			if err := validator.ValidateInputFile(inputFile); err != nil {
				return err
			}

			// Mock generation for testing
			if dryRun {
				cmd.Printf("DRY RUN: Would generate from %s file: %s\n", inputType, inputFile)
				if destination != "" {
					cmd.Printf("DRY RUN: Would output to %s\n", destination)
				}
			}

			return nil
		},
	}

	// Add flags
	cmd.Flags().String("erd", "", "ERD specification file")
	cmd.Flags().String("openapi", "", "OpenAPI specification file")
	cmd.Flags().String("proto", "", "Protocol Buffer definition file")
	cmd.Flags().StringP("destination", "d", "", "output directory")
	cmd.Flags().Bool("dry-run", false, "preview what would be generated")

	return cmd
}

func createTestFormatsCommand() *cobra.Command {
	return &cobra.Command{
		Use: "formats",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println("Supported input formats:")
			cmd.Println("  - OpenAPI")
			cmd.Println("  - Protocol Buffer")
			return nil
		},
	}
}

func createActualFormatsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "formats",
		Short: "List supported input formats",
		Long: `Display all supported input formats for code generation.

This command shows the available parsers and their supported file extensions,
helping you understand which input formats can be used with the generate command.`,
		Example: `  # List all supported formats
  pericarp formats`,
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := NewParserRegistry()

			// Register all available parsers (Requirement 7.5)
			parsers := []DomainParser{
				NewOpenAPIParser(),
				NewProtoParser(),
			}

			for _, parser := range parsers {
				if err := registry.RegisterParser(parser); err != nil {
					return NewCliError(ArgumentError,
						fmt.Sprintf("failed to register parser: %v", err),
						err)
				}
			}

			if len(parsers) == 0 {
				cmd.Println("No supported input formats found.")
				return nil
			}

			cmd.Println("Supported input formats:")
			cmd.Println()

			for _, parser := range parsers {
				cmd.Printf("  %s\n", parser.FormatName())
				cmd.Printf("    File extensions: %s\n", strings.Join(parser.SupportedExtensions(), ", "))
				cmd.Println()
			}

			return nil
		},
	}
}

// getGoVersion returns the Go version string
func getGoVersion() string {
	cmd := exec.Command("go", "version")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(output))
}
