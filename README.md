# Apicurio Registry CLI Client

[![CI](https://github.com/ymocode/apicurio-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/ymocode/apicurio-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ymocode/apicurio-cli?logo=github&sort=semver)](https://github.com/ymocode/apicurio-cli/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/ymocode/apicurio-cli?logo=go)](https://go.dev/)
[![Go Report Card](https://goreportcard.com/badge/github.com/ymocode/apicurio-cli)](https://goreportcard.com/report/github.com/ymocode/apicurio-cli)
[![GoDoc](https://pkg.go.dev/badge/github.com/ymocode/apicurio-cli)](https://pkg.go.dev/github.com/ymocode/apicurio-cli)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Production-ready Go CLI for Apicurio Registry 3.x with semantic versioning, batch operations, and multi-API support.

## Features

- **Multi-API Support** - V2, V3, and Confluent-compatible (CCOMPAT) APIs
- **Official SDK** - Uses Apicurio Registry Go SDK with Kiota framework
- **Semantic Versioning** - Automatic version calculation based on schema changes
- **Batch Operations** - Process multiple schemas with parallel workers
- **AsyncAPI Support** - Register AsyncAPI documents with schema references (V3 only)
- **Multiple Output Formats** - JSON, table, summary, and markdown reports
- **Authentication** - None, Basic Auth, and OIDC (Keycloak)
- **TLS/HTTPS** - Full TLS 1.2+ support with certificate validation
- **Enhanced Error Handling** - Clear timeout detection and detailed API error messages

## Quick Start

```bash
# Build
make build

# Get registry information
./bin/apicurio-client info --registry-url http://localhost:8081

# Register a schema
./bin/apicurio-client register \
  --registry-url http://localhost:8081 \
  --file schema.avsc

# Validate schema compatibility
./bin/apicurio-client validate \
  --registry-url http://localhost:8081 \
  --file schema-v2.avsc

# Batch validate multiple schemas
./bin/apicurio-client batch validate \
  --registry-url http://localhost:8081 \
  --dir ./schemas \
  --format summary

# Register AsyncAPI document (V3 only)
./bin/apicurio-client asyncapi register \
  --registry-url http://localhost:8081 \
  --api-version v3 \
  --file asyncapi.yaml

# Get dereferenced AsyncAPI document (V3 only)
./bin/apicurio-client asyncapi get \
  --registry-url http://localhost:8081 \
  --api-version v3 \
  --group com.example \
  --artifact-id my-api
```

## Commands

### info
Get registry system information and health status.

```bash
./bin/apicurio-client info \
  --registry-url http://localhost:8081 \
  --format markdown
```

### register
Register a new schema or create a new version.

```bash
./bin/apicurio-client register \
  --registry-url http://localhost:8081 \
  --api-version v3 \
  --file schema.avsc \
  --format table
```

**Flags:**
- `--dry-run` - Preview registration without actually registering
- `--skip-validation` - Skip validation before registration
- `--format` - Output format: json, table, summary, markdown

### validate
Validate schema compatibility without registration (dry-run validation).

```bash
./bin/apicurio-client validate \
  --registry-url http://localhost:8081 \
  --file schema.avsc \
  --format summary
```

### latest
Retrieve the latest version of a schema.

```bash
./bin/apicurio-client latest \
  --registry-url http://localhost:8081 \
  --namespace com.example \
  --name User
```

### batch validate
Validate multiple schemas in a directory.

```bash
./bin/apicurio-client batch validate \
  --registry-url http://localhost:8081 \
  --dir ./schemas \
  --pattern "*.avsc" \
  --parallel 4 \
  --format summary
```

**Flags:**
- `--dir` - Directory to scan (default: current directory)
- `--pattern` - File pattern (default: `*.avsc`)
- `--recursive` - Scan subdirectories (default: true)
- `--parallel` - Number of parallel workers (default: 4)
- `--continue-on-error` - Continue processing on failures

### batch register
Register multiple schemas to the registry.

```bash
./bin/apicurio-client batch register \
  --registry-url http://localhost:8081 \
  --dir ./schemas \
  --dry-run
```

### asyncapi validate
Validate an AsyncAPI document against the registry (V3 only).

```bash
./bin/apicurio-client asyncapi validate \
  --registry-url http://localhost:8081 \
  --api-version v3 \
  --file asyncapi.yaml
```

### asyncapi register
Register an AsyncAPI document with schema references (V3 only).

```bash
./bin/apicurio-client asyncapi register \
  --registry-url http://localhost:8081 \
  --api-version v3 \
  --file asyncapi.yaml
```

**Flags:**
- `--version` - Override version from document
- `--skip-validation` - Skip validation before registration

### asyncapi get
Retrieve a dereferenced AsyncAPI document from the registry (V3 only).

```bash
./bin/apicurio-client asyncapi get \
  --registry-url http://localhost:8081 \
  --api-version v3 \
  --group com.example \
  --artifact-id my-api
```

**Flags:**
- `--version` - Version to retrieve (default: branch=latest)
- `--format` - Output format: json, yaml (default: yaml)
- `--no-fix` - Disable automatic Avro schema wrapping fix

## Output Formats

All commands support multiple output formats via the `--format` flag:

**JSON** (default) - Machine-readable structured output
```bash
./bin/apicurio-client register --file schema.avsc --format json
```

**Table** - Human-readable formatted table
```bash
./bin/apicurio-client register --file schema.avsc --format table
```

**Summary** - Concise single-line output
```bash
./bin/apicurio-client register --file schema.avsc --format summary
```

**Markdown** - Professional markdown reports
```bash
./bin/apicurio-client register --file schema.avsc --format markdown -o report.md
```

## Configuration

Configuration priority: CLI flags > Environment variables > Config file

### Config File
Create `~/.apicurio-client.yaml`:

```yaml
registry_url: http://localhost:8081
api_version: v3

# Authentication
auth: oidc
keycloak_url: https://keycloak.example.com
client_id: apicurio-client
client_secret: your-secret
realm: apicurio

# TLS
insecure: false
```

### Environment Variables
```bash
# Connection
export APICURIO_REGISTRY_URL=http://localhost:8081
export APICURIO_API_VERSION=v3

# Schema defaults
export APICURIO_GROUP=default
export APICURIO_ARTIFACT_ID=MySchema

# Basic auth
export APICURIO_AUTH=basic
export APICURIO_USERNAME=admin
export APICURIO_PASSWORD=secret

# OIDC auth
export APICURIO_AUTH=oidc
export APICURIO_KEYCLOAK_URL=https://keycloak.example.com
export APICURIO_CLIENT_ID=apicurio-client
export APICURIO_CLIENT_SECRET=your-secret
export APICURIO_REALM=apicurio

# TLS
export APICURIO_INSECURE=true

# Logging
export APICURIO_VERBOSE=true
export APICURIO_DEBUG=true
```

## Authentication

### Basic Auth
```bash
./bin/apicurio-client register \
  --auth basic \
  --username admin \
  --password secret \
  --file schema.avsc
```

### OIDC (Keycloak)
```bash
./bin/apicurio-client register \
  --auth oidc \
  --keycloak-url https://keycloak.example.com \
  --client-id apicurio-client \
  --client-secret your-secret \
  --realm apicurio \
  --file schema.avsc
```

## API Versions

Select API version with `--api-version` flag:

- **v2** (default) - Stable, production-ready, official SDK
- **v3** - Latest features, improved API structure, official SDK
- **ccompat** - Confluent Schema Registry compatibility

```bash
# V3 API
./bin/apicurio-client register --api-version v3 --file schema.avsc

# CCOMPAT API
./bin/apicurio-client register --api-version ccompat --file schema.avsc
```

## Schema Requirements

Avro schemas must include:
- `namespace` - Schema namespace (e.g., "com.example")
- `name` - Schema name (e.g., "User")
- `version` - Semantic version (e.g., "1.0.0")

**Example:**
```json
{
  "type": "record",
  "namespace": "com.example",
  "name": "User",
  "version": "1.0.0",
  "fields": [
    {"name": "id", "type": "string"},
    {"name": "email", "type": ["null", "string"], "default": null}
  ]
}
```

## Semantic Versioning

Version bumps are calculated automatically based on changes:

| Change Type | Example | Version Bump |
|-------------|---------|--------------|
| **Patch** | Doc updates, metadata | 1.0.0 → 1.0.1 |
| **Minor** | Add optional field | 1.0.0 → 1.1.0 |
| **Major** | Remove field, change type | 1.0.0 → 2.0.0 |

The `validate` command detects version mismatches and suggests corrections.

## Project Structure

```
apicurio-client/
├── cmd/apicurio-client/    # Entry point
│   └── main.go
├── internal/
│   ├── cli/                # Cobra commands (register, validate, info, batch, asyncapi)
│   ├── operations/         # Business logic (registration, validation)
│   ├── registry/           # Registry clients (V2, V3, CCOMPAT)
│   ├── schema/             # Avro schema parsing, diffing, versioning
│   ├── asyncapi/           # AsyncAPI document parsing and validation
│   ├── batch/              # Batch processing with parallel workers
│   ├── output/             # Output formatters (JSON, table, markdown)
│   ├── templates/          # Embedded markdown templates
│   ├── config/             # Configuration management
│   ├── logger/             # Logging
│   └── auth/               # Authentication (Basic, OIDC)
├── Makefile                # Build and CI targets
├── Dockerfile              # Container image
└── .github/workflows/      # CI/CD pipelines
```

## Error Handling

Enhanced error messages with context and timeout detection:

**Timeout errors:**
```
[ERROR] failed to create artifact: operation timed out after exceeding context deadline (group=default, artifactId=User)
```

**API errors:**
```
[ERROR] failed to create artifact: HTTP 409 - Conflict: Artifact already exists (group=default, artifactId=User)
```

**Network errors:**
```
[ERROR] failed to create artifact: network timeout error (group=default, artifactId=User)
```

## TLS Configuration

For self-signed certificates:

```bash
./bin/apicurio-client register --insecure --registry-url https://... --file schema.avsc
```

Or in config:
```yaml
insecure: true
```

## Build and Development

```bash
# Download dependencies
go mod download

# Build
make build

# Run tests
make test

# Run linting
make lint

# Run all CI checks
make ci

# Build for all platforms
make build-all
```

## CI/CD

Automated workflows included:
- **CI**: Linting, testing, building (Linux, macOS, Windows)
- **Release**: Multi-platform binaries, checksums, Docker images

Create release:
```bash
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

## Dependencies

- `github.com/apicurio/apicurio-registry/go-sdk/v3` - Official Apicurio SDK
- `github.com/microsoft/kiota-abstractions-go` - Kiota framework
- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Configuration management
- `github.com/Masterminds/semver/v3` - Semantic versioning
- `golang.org/x/oauth2` - OAuth2/OIDC authentication
- `gopkg.in/yaml.v3` - YAML parsing (AsyncAPI)

## License

MIT License - see [LICENSE](LICENSE) for details.

