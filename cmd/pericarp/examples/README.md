# Pericarp CLI Examples

This directory contains example input files for testing and demonstrating the Pericarp CLI Generator.

## Example Files

### OpenAPI Specifications

- `user-service.yaml` - Complete user service API with CRUD operations
- `order-service.yaml` - E-commerce order service with complex relationships
- `simple-api.yaml` - Minimal API example for getting started

### Protocol Buffer Definitions

- `user.proto` - User service with basic CRUD operations
- `order.proto` - Order service with complex message types
- `simple.proto` - Minimal proto example for getting started

## Usage Examples

### Generate from OpenAPI

```bash
# Generate user service from OpenAPI spec
pericarp generate --openapi examples/user-service.yaml --destination ./user-service

# Preview generation
pericarp generate --openapi examples/user-service.yaml --dry-run --verbose
```

### Generate from Protocol Buffers

```bash
# Generate user service from proto definition
pericarp generate --proto examples/user.proto --destination ./user-service

# Preview generation
pericarp generate --proto examples/user.proto --dry-run --verbose
```

### Create New Projects

```bash
# Create a new project and then generate code
pericarp new user-service
cd user-service
pericarp generate --openapi ../examples/user-service.yaml
```

## Testing the Examples

All example files are designed to be valid and should generate working code:

```bash
# Test all OpenAPI examples
for file in examples/*.yaml; do
    echo "Testing $file"
    pericarp generate --openapi "$file" --dry-run
done

# Test all Proto examples
for file in examples/*.proto; do
    echo "Testing $file"
    pericarp generate --proto "$file" --dry-run
done
```