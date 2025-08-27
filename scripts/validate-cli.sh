#!/bin/bash

# Comprehensive validation script for Pericarp CLI
# Tests complete workflow from installation to code generation

set -e

echo "=================================================="
echo " PERICARP CLI END-TO-END VALIDATION"
echo "=================================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0
TOTAL_TESTS=0

# Helper functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

test_passed() {
    TESTS_PASSED=$((TESTS_PASSED + 1))
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    log_info "✓ $1"
}

test_failed() {
    TESTS_FAILED=$((TESTS_FAILED + 1))
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    log_error "✗ $1"
}

# Cleanup function
cleanup() {
    log_info "Cleaning up test artifacts..."
    rm -rf /tmp/pericarp-validation-*
}

# Set up cleanup trap (but not on EXIT to avoid interfering with exit codes)
# We'll call cleanup manually at the end

# Test 1: CLI Binary Exists and is Executable
echo ""
echo "Test 1: CLI Binary Validation"
echo "------------------------------"

if [ -f "./bin/pericarp" ]; then
    test_passed "CLI binary exists"
else
    test_failed "CLI binary does not exist"
    exit 1
fi

if [ -x "./bin/pericarp" ]; then
    test_passed "CLI binary is executable"
else
    test_failed "CLI binary is not executable"
    exit 1
fi

# Test 2: Version Information
echo ""
echo "Test 2: Version Information"
echo "---------------------------"

VERSION_OUTPUT=$(./bin/pericarp version 2>&1)
if echo "$VERSION_OUTPUT" | grep -q "Pericarp CLI Generator"; then
    test_passed "Version command shows correct title"
else
    test_failed "Version command output incorrect"
fi

if echo "$VERSION_OUTPUT" | grep -q "Version:"; then
    test_passed "Version information includes version"
else
    test_failed "Version information missing version"
fi

if echo "$VERSION_OUTPUT" | grep -q "Commit:"; then
    test_passed "Version information includes commit"
else
    test_failed "Version information missing commit"
fi

# Test 3: Help System
echo ""
echo "Test 3: Help System"
echo "-------------------"

HELP_OUTPUT=$(./bin/pericarp --help 2>&1)
if echo "$HELP_OUTPUT" | grep -q "Available Commands:"; then
    test_passed "Help shows available commands"
else
    test_failed "Help does not show available commands"
fi

if echo "$HELP_OUTPUT" | grep -q "new.*Create a new Pericarp project"; then
    test_passed "Help shows new command"
else
    test_failed "Help missing new command"
fi

if echo "$HELP_OUTPUT" | grep -q "generate.*Generate Pericarp code"; then
    test_passed "Help shows generate command"
else
    test_failed "Help missing generate command"
fi

# Test 4: Error Handling
echo ""
echo "Test 4: Error Handling"
echo "----------------------"

# Test invalid project name
if ./bin/pericarp new "" 2>&1 | grep -q "project name cannot be empty"; then
    test_passed "Empty project name validation"
else
    test_failed "Empty project name validation"
fi

# Test invalid project name format
if ./bin/pericarp new "My-Service" 2>&1 | grep -q "must start with lowercase letter"; then
    test_passed "Invalid project name format validation"
else
    test_failed "Invalid project name format validation"
fi

# Test missing input file
if ./bin/pericarp generate --openapi nonexistent.yaml 2>&1 | grep -q "input file does not exist"; then
    test_passed "Missing input file validation"
else
    test_failed "Missing input file validation"
fi

# Test missing input format
if ./bin/pericarp generate 2>&1 | grep -q "must specify exactly one input format"; then
    test_passed "Missing input format validation"
else
    test_failed "Missing input format validation"
fi

# Test 5: Supported Formats
echo ""
echo "Test 5: Supported Formats"
echo "-------------------------"

FORMATS_OUTPUT=$(./bin/pericarp formats 2>&1)
if echo "$FORMATS_OUTPUT" | grep -q "OpenAPI"; then
    test_passed "Formats command shows OpenAPI support"
else
    test_failed "Formats command missing OpenAPI support"
fi

if echo "$FORMATS_OUTPUT" | grep -q "Protocol Buffers"; then
    test_passed "Formats command shows Protocol Buffers support"
else
    test_failed "Formats command missing Protocol Buffers support"
fi

# Test 6: Project Creation
echo ""
echo "Test 6: Project Creation"
echo "------------------------"

TEST_DIR="/tmp/pericarp-validation-$$"
mkdir -p "$TEST_DIR"

# Test dry-run project creation
if ./bin/pericarp new test-service --destination "$TEST_DIR" --dry-run 2>&1 | grep -q "Dry run completed"; then
    test_passed "Project creation dry-run works"
else
    test_failed "Project creation dry-run failed"
fi

# Test actual project creation
if ./bin/pericarp new test-service --destination "$TEST_DIR" 2>&1 | grep -q "Successfully created project"; then
    test_passed "Project creation works"
else
    test_failed "Project creation failed"
fi

# Verify project structure
PROJECT_PATH="$TEST_DIR"
if [ -f "$PROJECT_PATH/go.mod" ]; then
    test_passed "Generated project has go.mod"
else
    test_failed "Generated project missing go.mod"
fi

if [ -f "$PROJECT_PATH/README.md" ]; then
    test_passed "Generated project has README.md"
else
    test_failed "Generated project missing README.md"
fi

if [ -d "$PROJECT_PATH/internal/domain" ]; then
    test_passed "Generated project has domain directory"
else
    test_failed "Generated project missing domain directory"
fi

if [ -d "$PROJECT_PATH/internal/application" ]; then
    test_passed "Generated project has application directory"
else
    test_failed "Generated project missing application directory"
fi

if [ -d "$PROJECT_PATH/internal/infrastructure" ]; then
    test_passed "Generated project has infrastructure directory"
else
    test_failed "Generated project missing infrastructure directory"
fi

# Test 7: Code Generation from OpenAPI
echo ""
echo "Test 7: Code Generation from OpenAPI"
echo "------------------------------------"

# Test dry-run code generation
if ./bin/pericarp generate --openapi cmd/pericarp/examples/simple-api.yaml --destination "$PROJECT_PATH" --dry-run 2>&1 | grep -q "Dry run completed"; then
    test_passed "OpenAPI code generation dry-run works"
else
    test_failed "OpenAPI code generation dry-run failed"
fi

# Test actual code generation
if ./bin/pericarp generate --openapi cmd/pericarp/examples/simple-api.yaml --destination "$PROJECT_PATH" 2>&1 | grep -q "Successfully generated"; then
    test_passed "OpenAPI code generation works"
else
    test_failed "OpenAPI code generation failed"
fi

# Verify generated files
if [ -f "$PROJECT_PATH/internal/domain/product.go" ]; then
    test_passed "Generated domain entity exists"
else
    test_failed "Generated domain entity missing"
fi

if [ -f "$PROJECT_PATH/internal/domain/product_repository.go" ]; then
    test_passed "Generated repository interface exists"
else
    test_failed "Generated repository interface missing"
fi

if [ -f "$PROJECT_PATH/internal/infrastructure/product_repository.go" ]; then
    test_passed "Generated repository implementation exists"
else
    test_failed "Generated repository implementation missing"
fi

if [ -f "$PROJECT_PATH/internal/application/product_commands.go" ]; then
    test_passed "Generated commands exist"
else
    test_failed "Generated commands missing"
fi

if [ -f "$PROJECT_PATH/internal/application/product_command_handlers.go" ]; then
    test_passed "Generated command handlers exist"
else
    test_failed "Generated command handlers missing"
fi

# Test 8: Code Generation from Protocol Buffers
echo ""
echo "Test 8: Code Generation from Protocol Buffers"
echo "----------------------------------------------"

TEST_DIR_PROTO="/tmp/pericarp-validation-proto-$$"
mkdir -p "$TEST_DIR_PROTO"

# Create a new project for proto testing
./bin/pericarp new proto-service --destination "$TEST_DIR_PROTO" > /dev/null 2>&1

# Test proto code generation
if ./bin/pericarp generate --proto cmd/pericarp/examples/simple.proto --destination "$TEST_DIR_PROTO" 2>&1 | grep -q "Successfully generated"; then
    test_passed "Protocol Buffers code generation works"
else
    test_failed "Protocol Buffers code generation failed"
fi

# Test 9: Generated Code Quality
echo ""
echo "Test 9: Generated Code Quality"
echo "------------------------------"

# Check if generated code follows Go conventions
if grep -q "package domain" "$PROJECT_PATH/internal/domain/product.go"; then
    test_passed "Generated code has correct package declaration"
else
    test_failed "Generated code has incorrect package declaration"
fi

# Check if generated code includes proper imports
if grep -q "github.com/google/uuid" "$PROJECT_PATH/internal/domain/product.go"; then
    test_passed "Generated code includes required imports"
else
    test_failed "Generated code missing required imports"
fi

# Check if generated code includes validation tags
if grep -q "validate:" "$PROJECT_PATH/internal/domain/product.go"; then
    test_passed "Generated code includes validation tags"
else
    test_failed "Generated code missing validation tags"
fi

# Test 10: Example Files Validation
echo ""
echo "Test 10: Example Files Validation"
echo "---------------------------------"

# Test all example OpenAPI files
for example_file in cmd/pericarp/examples/*.yaml; do
    if [ -f "$example_file" ]; then
        filename=$(basename "$example_file")
        if ./bin/pericarp generate --openapi "$example_file" --dry-run > /dev/null 2>&1; then
            test_passed "Example file $filename is valid"
        else
            test_failed "Example file $filename is invalid"
        fi
    fi
done

# Test all example Proto files
for example_file in cmd/pericarp/examples/*.proto; do
    if [ -f "$example_file" ]; then
        filename=$(basename "$example_file")
        if ./bin/pericarp generate --proto "$example_file" --dry-run > /dev/null 2>&1; then
            test_passed "Example file $filename is valid"
        else
            test_failed "Example file $filename is invalid"
        fi
    fi
done

# Test 11: Verbose Output
echo ""
echo "Test 11: Verbose Output"
echo "----------------------"

if ./bin/pericarp new verbose-test --destination "/tmp/pericarp-validation-verbose-$$" --dry-run --verbose 2>&1 | grep -q "\[DEBUG\]"; then
    test_passed "Verbose output works"
else
    test_failed "Verbose output not working"
fi

# Test 12: Exit Codes
echo ""
echo "Test 12: Exit Codes"
echo "-------------------"

# Test successful command (exit code 0)
./bin/pericarp --help > /dev/null 2>&1
if [ $? -eq 0 ]; then
    test_passed "Successful command returns exit code 0"
else
    test_failed "Successful command returns wrong exit code"
fi

# Test validation error (exit code 3)
./bin/pericarp new "" > /dev/null 2>&1
EXIT_CODE=$?
if [ $EXIT_CODE -eq 3 ]; then
    test_passed "Validation error returns exit code 3"
else
    test_failed "Validation error returns wrong exit code (got $EXIT_CODE, expected 3)"
fi

# Test file system error (exit code 6)
./bin/pericarp generate --openapi nonexistent.yaml > /dev/null 2>&1
EXIT_CODE=$?
if [ $EXIT_CODE -eq 6 ]; then
    test_passed "File system error returns exit code 6"
else
    test_failed "File system error returns wrong exit code (got $EXIT_CODE, expected 6)"
fi

# Final Results
echo ""
echo "=================================================="
echo " VALIDATION RESULTS"
echo "=================================================="
echo "Total Tests: $TOTAL_TESTS"
echo "Passed: $TESTS_PASSED"
echo "Failed: $TESTS_FAILED"

# Cleanup before final results
cleanup

if [ $TESTS_FAILED -eq 0 ]; then
    log_info "🎉 ALL TESTS PASSED! CLI is ready for production."
    exit 0
else
    log_error "❌ $TESTS_FAILED tests failed. Please review and fix issues."
    exit 1
fi