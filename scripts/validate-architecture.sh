#!/bin/bash

# Architecture validation script for Pericarp Go library
# This script validates that clean architecture boundaries are maintained

set -e

echo "🏗️  Validating Clean Architecture Boundaries..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    local color=$1
    local message=$2
    echo -e "${color}${message}${NC}"
}

# Function to check for forbidden imports
check_forbidden_imports() {
    local layer=$1
    local forbidden_patterns=("${@:2}")
    local violations=0
    
    print_status $YELLOW "Checking $layer layer dependencies..."
    
    for pattern in "${forbidden_patterns[@]}"; do
        local files=$(find "pkg/$layer" -name "*.go" -exec grep -l "$pattern" {} \; 2>/dev/null || true)
        if [ ! -z "$files" ]; then
            print_status $RED "❌ $layer layer violation: Found forbidden import '$pattern' in:"
            echo "$files" | sed 's/^/  /'
            violations=$((violations + 1))
        fi
    done
    
    if [ $violations -eq 0 ]; then
        print_status $GREEN "✅ $layer layer dependencies are clean"
    fi
    
    return $violations
}

# Function to check for reflection usage in hot paths
check_reflection_usage() {
    print_status $YELLOW "Checking for reflection usage in hot paths..."
    
    local reflection_files=$(find pkg/ -name "*.go" -exec grep -l "reflect\." {} \; 2>/dev/null || true)
    local violations=0
    
    if [ ! -z "$reflection_files" ]; then
        print_status $YELLOW "⚠️  Found reflection usage in:"
        echo "$reflection_files" | sed 's/^/  /'
        
        # Check if reflection is in performance-critical paths
        for file in $reflection_files; do
            if grep -q "func.*Handle\|func.*Save\|func.*Load" "$file"; then
                print_status $RED "❌ Reflection found in performance-critical path: $file"
                violations=$((violations + 1))
            fi
        done
    fi
    
    if [ $violations -eq 0 ]; then
        print_status $GREEN "✅ No reflection in performance-critical paths"
    fi
    
    return $violations
}

# Function to check for proper error handling
check_error_handling() {
    print_status $YELLOW "Checking error handling patterns..."
    
    local violations=0
    
    # Check for naked returns in error cases
    local naked_returns=$(find pkg/ -name "*.go" -exec grep -n "return$" {} \; | grep -v "_test.go" || true)
    if [ ! -z "$naked_returns" ]; then
        print_status $RED "❌ Found naked returns (should return explicit error):"
        echo "$naked_returns" | sed 's/^/  /'
        violations=$((violations + 1))
    fi
    
    # Check for proper error wrapping
    local unwrapped_errors=$(find pkg/ -name "*.go" -exec grep -n "return.*err$" {} \; | grep -v "fmt.Errorf\|errors.Wrap\|errors.WithMessage" | grep -v "_test.go" || true)
    if [ ! -z "$unwrapped_errors" ]; then
        print_status $YELLOW "⚠️  Consider wrapping these errors for better context:"
        echo "$unwrapped_errors" | head -10 | sed 's/^/  /'
        if [ $(echo "$unwrapped_errors" | wc -l) -gt 10 ]; then
            echo "  ... and $(( $(echo "$unwrapped_errors" | wc -l) - 10 )) more"
        fi
    fi
    
    if [ $violations -eq 0 ]; then
        print_status $GREEN "✅ Error handling patterns look good"
    fi
    
    return $violations
}

# Function to check for proper logging usage
check_logging_patterns() {
    print_status $YELLOW "Checking logging patterns..."
    
    local violations=0
    
    # Check for fmt.Print* usage instead of logger
    local fmt_prints=$(find pkg/ -name "*.go" -exec grep -n "fmt\.Print\|log\.Print" {} \; | grep -v "_test.go" || true)
    if [ ! -z "$fmt_prints" ]; then
        print_status $RED "❌ Found fmt.Print/log.Print usage (should use structured logger):"
        echo "$fmt_prints" | sed 's/^/  /'
        violations=$((violations + 1))
    fi
    
    if [ $violations -eq 0 ]; then
        print_status $GREEN "✅ Logging patterns are consistent"
    fi
    
    return $violations
}

# Function to check for performance anti-patterns
check_performance_patterns() {
    print_status $YELLOW "Checking for performance anti-patterns..."
    
    local violations=0
    
    # Check for string concatenation in loops
    local string_concat=$(find pkg/ -name "*.go" -exec grep -A5 -B5 "for.*{" {} \; | grep -n "+=" | grep -v "_test.go" || true)
    if [ ! -z "$string_concat" ]; then
        print_status $YELLOW "⚠️  Found potential string concatenation in loops (consider strings.Builder):"
        echo "$string_concat" | head -5 | sed 's/^/  /'
    fi
    
    # Check for inefficient JSON marshaling
    local json_marshal=$(find pkg/ -name "*.go" -exec grep -n "json\.Marshal.*json\.Marshal" {} \; | grep -v "_test.go" || true)
    if [ ! -z "$json_marshal" ]; then
        print_status $YELLOW "⚠️  Found multiple JSON marshal calls (consider batching):"
        echo "$json_marshal" | sed 's/^/  /'
    fi
    
    if [ $violations -eq 0 ]; then
        print_status $GREEN "✅ No obvious performance anti-patterns found"
    fi
    
    return $violations
}

# Main validation
total_violations=0

# Check domain layer purity (no infrastructure dependencies)
check_forbidden_imports "domain" \
    "gorm.io" \
    "database/sql" \
    "github.com/.*watermill" \
    "github.com/.*viper" \
    "go.uber.org/fx"
total_violations=$((total_violations + $?))

# Check application layer (no infrastructure implementations)
check_forbidden_imports "application" \
    "gorm.io" \
    "database/sql" \
    "github.com/.*watermill"
total_violations=$((total_violations + $?))

# Check for reflection usage
check_reflection_usage
total_violations=$((total_violations + $?))

# Check error handling
check_error_handling
total_violations=$((total_violations + $?))

# Check logging patterns
check_logging_patterns
total_violations=$((total_violations + $?))

# Check performance patterns
check_performance_patterns
total_violations=$((total_violations + $?))

# Final result
echo ""
if [ $total_violations -eq 0 ]; then
    print_status $GREEN "🎉 All architecture validations passed!"
    exit 0
else
    print_status $RED "❌ Found $total_violations architecture violations"
    exit 1
fi