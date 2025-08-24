#!/bin/bash

# Comprehensive Test Runner for Pericarp Go Library
# This script runs all tests including BDD scenarios, integration tests, and performance tests

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to run a test and capture results
run_test() {
    local test_name="$1"
    local test_command="$2"
    
    print_status "Running $test_name..."
    
    if eval "$test_command"; then
        print_success "$test_name passed"
        return 0
    else
        print_error "$test_name failed"
        return 1
    fi
}

# Initialize counters
total_tests=0
passed_tests=0
failed_tests=0

# Array to store failed test names
failed_test_names=()

# Function to update test counters
update_counters() {
    local result=$1
    local test_name="$2"
    
    total_tests=$((total_tests + 1))
    
    if [ $result -eq 0 ]; then
        passed_tests=$((passed_tests + 1))
    else
        failed_tests=$((failed_tests + 1))
        failed_test_names+=("$test_name")
    fi
}

echo "========================================"
echo "Pericarp Comprehensive Test Suite"
echo "========================================"

# Check if PostgreSQL is available
if [ -n "$POSTGRES_TEST_DSN" ]; then
    print_status "PostgreSQL test database configured: $POSTGRES_TEST_DSN"
    POSTGRES_AVAILABLE=true
else
    print_warning "PostgreSQL test database not configured (set POSTGRES_TEST_DSN to enable)"
    POSTGRES_AVAILABLE=false
fi

echo ""
print_status "Starting test execution..."
echo ""

# 1. Unit Tests
echo "----------------------------------------"
echo "1. UNIT TESTS"
echo "----------------------------------------"

run_test "Unit Tests" "go test -v -race -coverprofile=coverage.out ./pkg/... ./internal/..."
update_counters $? "Unit Tests"

# 2. BDD Tests
echo ""
echo "----------------------------------------"
echo "2. BDD TESTS"
echo "----------------------------------------"

run_test "BDD User Management Tests" "go test -v ./test/bdd/user_management_test.go"
update_counters $? "BDD User Management Tests"

run_test "BDD SQLite Database Tests" "go test -v ./test/bdd/database_sqlite_test.go"
update_counters $? "BDD SQLite Database Tests"

if [ "$POSTGRES_AVAILABLE" = true ]; then
    run_test "BDD PostgreSQL Database Tests" "go test -v ./test/bdd/database_postgres_test.go"
    update_counters $? "BDD PostgreSQL Database Tests"
else
    print_warning "Skipping BDD PostgreSQL tests (POSTGRES_TEST_DSN not set)"
fi

# 3. Integration Tests
echo ""
echo "----------------------------------------"
echo "3. INTEGRATION TESTS"
echo "----------------------------------------"

run_test "EventStore Integration Tests" "go test -v -tags=integration ./test/integration/eventstore_integration_test.go"
update_counters $? "EventStore Integration Tests"

run_test "EventDispatcher Integration Tests" "go test -v -tags=integration ./test/integration/eventdispatcher_integration_test.go"
update_counters $? "EventDispatcher Integration Tests"

run_test "End-to-End Integration Tests" "go test -v -tags=integration ./test/integration/end_to_end_test.go"
update_counters $? "End-to-End Integration Tests"

# 4. Performance Tests (only if not in short mode)
if [ "$1" != "--short" ]; then
    echo ""
    echo "----------------------------------------"
    echo "4. PERFORMANCE TESTS"
    echo "----------------------------------------"
    
    run_test "Performance and Concurrency Tests" "go test -v -tags=integration ./test/integration/performance_test.go -timeout=5m"
    update_counters $? "Performance and Concurrency Tests"
else
    print_warning "Skipping performance tests (use without --short to include)"
fi

# 5. Feature File Validation
echo ""
echo "----------------------------------------"
echo "5. FEATURE FILE VALIDATION"
echo "----------------------------------------"

run_test "Feature File Validation" "find features -name '*.feature' -exec echo 'Validating {}' \; -exec grep -q 'Feature:' {} \;"
update_counters $? "Feature File Validation"

# 6. Code Quality Checks
echo ""
echo "----------------------------------------"
echo "6. CODE QUALITY CHECKS"
echo "----------------------------------------"

# Check if golangci-lint is available
if command -v golangci-lint &> /dev/null; then
    run_test "Linting" "golangci-lint run"
    update_counters $? "Linting"
else
    print_warning "golangci-lint not found, skipping linting"
fi

# Check formatting
run_test "Code Formatting Check" "test -z \$(gofmt -l .)"
update_counters $? "Code Formatting Check"

# Check for go mod tidy
run_test "Go Mod Tidy Check" "go mod tidy && git diff --exit-code go.mod go.sum"
update_counters $? "Go Mod Tidy Check"

# 7. Security Scan
echo ""
echo "----------------------------------------"
echo "7. SECURITY SCAN"
echo "----------------------------------------"

if command -v gosec &> /dev/null; then
    run_test "Security Scan" "gosec ./..."
    update_counters $? "Security Scan"
else
    print_warning "gosec not found, skipping security scan"
fi

# 8. Test Coverage Analysis
echo ""
echo "----------------------------------------"
echo "8. TEST COVERAGE ANALYSIS"
echo "----------------------------------------"

if [ -f coverage.out ]; then
    print_status "Generating coverage report..."
    go tool cover -html=coverage.out -o coverage.html
    
    # Extract coverage percentage
    coverage_percent=$(go tool cover -func=coverage.out | grep total | awk '{print $3}')
    print_status "Total test coverage: $coverage_percent"
    
    # Check if coverage meets minimum threshold
    coverage_num=$(echo $coverage_percent | sed 's/%//')
    min_coverage=70
    
    if (( $(echo "$coverage_num >= $min_coverage" | bc -l) )); then
        print_success "Coverage meets minimum threshold ($min_coverage%)"
    else
        print_warning "Coverage below minimum threshold: $coverage_percent < $min_coverage%"
    fi
else
    print_warning "No coverage file found"
fi

# Final Results
echo ""
echo "========================================"
echo "TEST EXECUTION SUMMARY"
echo "========================================"

print_status "Total tests run: $total_tests"
print_success "Passed: $passed_tests"

if [ $failed_tests -gt 0 ]; then
    print_error "Failed: $failed_tests"
    echo ""
    print_error "Failed tests:"
    for test_name in "${failed_test_names[@]}"; do
        echo "  - $test_name"
    done
    echo ""
    print_error "Some tests failed. Please check the output above for details."
    exit 1
else
    echo ""
    print_success "All tests passed! 🎉"
    
    if [ -f coverage.html ]; then
        print_status "Coverage report generated: coverage.html"
    fi
    
    echo ""
    print_success "The Pericarp Go library is ready for use!"
fi

echo "========================================"