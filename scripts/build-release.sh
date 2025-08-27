#!/bin/bash

# Cross-platform build script for Pericarp CLI releases
# Builds binaries for multiple platforms

set -e

# Get build information
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}
COMMIT=${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "none")}
DATE=${DATE:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}
GO_VERSION=${GO_VERSION:-$(go version | awk '{print $3}')}
BUILT_BY=${BUILT_BY:-"release-script"}

# Build flags
LDFLAGS="-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE} -X main.goVersion=${GO_VERSION} -X main.builtBy=${BUILT_BY}"

# Output directory
OUTPUT_DIR=${OUTPUT_DIR:-"dist"}
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

# Platforms to build for
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
    "windows/arm64"
)

echo "Building Pericarp CLI for multiple platforms..."
echo "Version: ${VERSION}"
echo "Commit: ${COMMIT}"
echo "Date: ${DATE}"
echo "Go Version: ${GO_VERSION}"
echo "Built By: ${BUILT_BY}"
echo ""

for platform in "${PLATFORMS[@]}"; do
    IFS='/' read -r GOOS GOARCH <<< "${platform}"
    
    output_name="pericarp"
    if [ "${GOOS}" = "windows" ]; then
        output_name="pericarp.exe"
    fi
    
    output_path="${OUTPUT_DIR}/${GOOS}-${GOARCH}/${output_name}"
    mkdir -p "$(dirname "${output_path}")"
    
    echo "Building for ${GOOS}/${GOARCH}..."
    
    env GOOS="${GOOS}" GOARCH="${GOARCH}" go build \
        -ldflags "${LDFLAGS}" \
        -o "${output_path}" \
        ./cmd/pericarp
    
    # Create archive
    archive_name="pericarp-${VERSION}-${GOOS}-${GOARCH}"
    if [ "${GOOS}" = "windows" ]; then
        archive_name="${archive_name}.zip"
        (cd "${OUTPUT_DIR}/${GOOS}-${GOARCH}" && zip -r "../${archive_name}" .)
    else
        archive_name="${archive_name}.tar.gz"
        (cd "${OUTPUT_DIR}/${GOOS}-${GOARCH}" && tar -czf "../${archive_name}" .)
    fi
    
    echo "Created: ${OUTPUT_DIR}/${archive_name}"
done

echo ""
echo "Cross-platform build complete!"
echo "Artifacts in: ${OUTPUT_DIR}/"
ls -la "${OUTPUT_DIR}/"