package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/akeemphilbert/pericarp/pkg/domain"
)

func TestDefaultCliLogger_ImplementsDomainLogger(t *testing.T) {
	logger := NewCliLogger()

	// Verify that CliLogger implements domain.Logger interface
	var _ domain.Logger = logger

	// Verify that it also implements CliLogger interface
	var _ CliLogger = logger
}

func TestDefaultCliLogger_VerboseMode(t *testing.T) {
	logger := NewCliLogger()

	// Initially verbose should be false
	if logger.IsVerbose() {
		t.Error("Expected verbose to be false initially")
	}

	// Enable verbose mode
	logger.SetVerbose(true)
	if !logger.IsVerbose() {
		t.Error("Expected verbose to be true after enabling")
	}

	// Disable verbose mode
	logger.SetVerbose(false)
	if logger.IsVerbose() {
		t.Error("Expected verbose to be false after disabling")
	}
}

func TestDefaultCliLogger_StructuredLogging(t *testing.T) {
	// Capture stdout for testing
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger := NewCliLogger()
	logger.Info("Test message", "key1", "value1", "key2", "value2")

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify structured logging format
	if !strings.Contains(output, "[INFO] Test message key1=value1 key2=value2") {
		t.Errorf("Expected structured logging format, got: %s", output)
	}
}

func TestDefaultCliLogger_FormattedLogging(t *testing.T) {
	// Capture stdout for testing
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger := NewCliLogger()
	logger.Infof("Test message with %s and %d", "string", 42)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify formatted logging
	if !strings.Contains(output, "[INFO] Test message with string and 42") {
		t.Errorf("Expected formatted logging, got: %s", output)
	}
}

func TestDefaultCliLogger_DebugVerboseMode(t *testing.T) {
	// Capture stdout for testing
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger := NewCliLogger()

	// Debug should not output when verbose is false
	logger.SetVerbose(false)
	logger.Debug("Debug message")

	// Enable verbose and try again
	logger.SetVerbose(true)
	logger.Debug("Debug message", "key", "value")

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Should only contain the verbose debug message
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Errorf("Expected only one debug line (verbose mode), got %d lines: %v", len(lines), lines)
	}

	if !strings.Contains(output, "[DEBUG] Debug message key=value") {
		t.Errorf("Expected debug message with structured logging, got: %s", output)
	}
}
