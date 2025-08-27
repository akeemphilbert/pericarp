#!/bin/bash

# Build script for Pericarp CLI with version information
# This script builds the CLI binary with proper version metadata

set -e

# Get build information
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}
COMMIT=${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "none")}
DATE=${DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}
GO_VERSION=${GO_VERSION:-$(go version | awk '{print $3}')}
BUILT_BY=${BUILT_BY:-$(whoami)}

# Build flags
LDFLAGS="-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE} -X main.goVersion=${GO_VERSION} -X main.builtBy=${BUILT_BY}"

# Output directory
OUTPUT_DIR=${OUTPUT_DIR:-"bin"}
mkdir -p "${OUTPUT_DIR}"

# Build for current platform
echo "Building Pericarp CLI..."
echo "Version: ${VERSION}"
echo "Commit: ${COMMIT}"
echo "Date: ${DATE}"
echo "Go Version: ${GO_VERSION}"
echo "Built By: ${BUILT_BY}"
echo ""

go build -ldflags "${LDFLAGS}" -o "${OUTPUT_DIR}/pericarp" ./cmd/pericarp

echo "Build complete: ${OUTPUT_DIR}/pericarp"

# Test the binary
echo ""
echo "Testing binary..."
"${OUTPUT_DIR}/pericarp" version