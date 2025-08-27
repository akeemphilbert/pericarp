package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

var (
	// Build information - set via ldflags during build
	version   = "dev"
	commit    = "none"
	date      = "unknown"
	goVersion = "unknown"
	builtBy   = "unknown"
)

// Global flags
var (
	verbose bool
	logger  CliLogger
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "pericarp",
	Short: "Pericarp CLI Generator for scaffolding DDD projects",
	Long: `Pericarp CLI Generator enables developers to scaffold new Pericarp-based projects
with automated code generation from various input formats including ERD, OpenAPI, and Protocol Buffers.

The CLI follows domain-driven design principles and generates production-ready code
with proper aggregate patterns, repositories, command/query handlers, and comprehensive testing.

Examples:
  # Create a new project
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
		// Initialize enhanced verbose logger with debug output to stdout (Requirement 9.2, 9.6)
		verboseLogger := NewVerboseLogger()
		verboseLogger.SetVerbose(verbose)
		if verbose {
			verboseLogger.SetTimestamp(true)
			verboseLogger.SetPrefix("CLI")
		}
		logger = verboseLogger
	},
	Run: func(cmd *cobra.Command, args []string) {
		// Show help when no command is provided (Requirement 1.3)
		cmd.Help()
	},
}

// newCmd creates a new Pericarp project
var newCmd = &cobra.Command{
	Use:   "new <project-name>",
	Short: "Create a new Pericarp project",
	Long: `Create a new Pericarp project with proper directory structure and boilerplate code.

This command scaffolds a new project following Pericarp conventions with:
- Go module setup with proper dependencies
- Domain-driven design directory structure
- Basic entity, repository, and handler templates
- Comprehensive Makefile with development targets
- Example configuration files

The project can be created from scratch or by cloning an existing repository
and adding Pericarp capabilities to it.`,
	Example: `  # Create a new project
  pericarp new my-service

  # Create a new project in a specific directory
  pericarp new my-service --destination /path/to/projects

  # Create a new project from existing repository
  pericarp new my-service --repo https://github.com/user/repo.git

  # Preview what would be created without actually creating files
  pericarp new my-service --dry-run

  # Create with verbose output
  pericarp new my-service --verbose`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectName := args[0]

		// Get flags
		repoURL, _ := cmd.Flags().GetString("repo")
		destination, _ := cmd.Flags().GetString("destination")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		// Validate project name (Requirement 10.1)
		validator := NewValidator()
		if err := validator.ValidateProjectName(projectName); err != nil {
			return err
		}

		// Validate destination if provided (Requirement 10.2)
		if destination != "" {
			if err := validator.ValidateDestination(destination); err != nil {
				return err
			}
		}

		// Create project
		creator := NewProjectCreator(logger)
		return creator.CreateProject(projectName, repoURL, destination, dryRun)
	},
}

// generateCmd generates code from various input formats
var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate Pericarp code from specifications",
	Long: `Generate Pericarp domain entities, repositories, handlers, and other components
from various input format specifications.

Supported input formats:
- ERD (Entity Relationship Diagrams) in YAML format
- OpenAPI 3.0 specifications in YAML or JSON
- Protocol Buffer definitions (.proto files)

The generated code follows Pericarp best practices and includes:
- Domain entities with aggregate root patterns
- Repository interfaces and implementations
- Command and query handlers with proper validation
- Domain events and event handlers
- Comprehensive unit tests`,
	Example: `  # Generate from ERD specification
  pericarp generate --erd entities.yaml

  # Generate from OpenAPI specification
  pericarp generate --openapi api.yaml

  # Generate from Protocol Buffer definition
  pericarp generate --proto user.proto

  # Generate to specific destination
  pericarp generate --openapi api.yaml --destination ./generated

  # Preview generation without creating files
  pericarp generate --erd entities.yaml --dry-run

  # Generate with verbose output
  pericarp generate --openapi api.yaml --verbose`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get input format flags (Requirement 3)
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
				"must specify exactly one input format (--erd, --openapi, or --proto)",
				nil)
		}
		if inputCount > 1 {
			return NewCliError(ArgumentError,
				"cannot specify multiple input formats simultaneously",
				nil)
		}

		// Validate input file exists (Requirement 10.2)
		validator := NewValidator()
		if err := validator.ValidateInputFile(inputFile); err != nil {
			return err
		}

		// Validate destination if provided (Requirement 10.3)
		if destination != "" {
			if err := validator.ValidateDestination(destination); err != nil {
				return err
			}
		}

		// Execute code generation
		generator := NewCodeGenerator(logger)
		return generator.Generate(inputFile, inputType, destination, dryRun)
	},
}

// formatsCmd lists supported input formats
var formatsCmd = &cobra.Command{
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
			fmt.Println("No supported input formats found.")
			return nil
		}

		fmt.Println("Supported input formats:")
		fmt.Println()

		for _, parser := range parsers {
			fmt.Printf("  %s\n", parser.FormatName())
			fmt.Printf("    File extensions: %s\n", strings.Join(parser.SupportedExtensions(), ", "))
			fmt.Println()
		}

		return nil
	},
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long: `Display version information including build details.

This command shows the current version of the Pericarp CLI tool,
along with commit hash, build date, and Go version for debugging purposes.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Pericarp CLI Generator\n")
		fmt.Printf("Version:    %s\n", version)
		fmt.Printf("Commit:     %s\n", commit)
		fmt.Printf("Built:      %s\n", date)
		fmt.Printf("Go version: %s\n", goVersion)
		fmt.Printf("Built by:   %s\n", builtBy)
		fmt.Printf("Platform:   %s\n", getPlatform())
	},
}

func init() {
	// Add global flags (Requirement 1.2)
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")

	// Add flags to new command (Requirement 9)
	newCmd.Flags().StringP("repo", "r", "", "Git repository URL to clone from")
	newCmd.Flags().StringP("destination", "d", "", "destination directory (defaults to project name)")
	newCmd.Flags().Bool("dry-run", false, "preview what would be created without actually creating files")

	// Add flags to generate command (Requirement 3, 9)
	generateCmd.Flags().String("erd", "", "ERD specification file (YAML format)")
	generateCmd.Flags().String("openapi", "", "OpenAPI specification file (YAML or JSON)")
	generateCmd.Flags().String("proto", "", "Protocol Buffer definition file (.proto)")
	generateCmd.Flags().StringP("destination", "d", "", "output directory for generated code")
	generateCmd.Flags().Bool("dry-run", false, "preview what would be generated without creating files")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(newCmd)
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(formatsCmd)

	// Customize help template for better usability (Requirement 1.3)
	rootCmd.SetHelpTemplate(`{{.Long}}

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
}

// getPlatform returns the current platform information
func getPlatform() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

func main() {
	// Enhanced error handling with proper exit codes
	if err := rootCmd.Execute(); err != nil {
		// Check if it's a CliError for proper exit code handling
		if cliErr, ok := err.(*CliError); ok {
			fmt.Fprintf(os.Stderr, "Error: %v\n", cliErr)
			os.Exit(cliErr.ExitCode())
		}

		// Default error handling
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
