# Config Module Design

**Status:** ✅ Complete

Production-ready configuration module for arrgo with config discovery, enhanced environment variable substitution, and validation.

## Config Discovery

Search order (first match wins):

1. Explicit path via `-config` flag or `ARRGO_CONFIG` env var
2. `./config.toml` (current directory)
3. `$XDG_CONFIG_HOME/arrgo/config.toml` (defaults to `~/.config/arrgo/`)
4. `/etc/arrgo/config.toml` (system-wide)

If `ARRGO_CONFIG` is set, use it directly and fail if not found. Otherwise, walk through the search order and return a clear error listing all locations checked.

## Environment Variable Substitution

Extended syntax:

| Pattern | Behavior |
|---------|----------|
| `${VAR}` | Substitute value, track as missing if not found |
| `${VAR:-default}` | Use default if VAR is unset or empty |
| `${VAR:?error message}` | Fail with custom error if VAR is unset |

Regex pattern:
```go
envVarPattern = regexp.MustCompile(`\$\{([^}:]+)(?:(:[-?])([^}]*))?\}`)
```

All unresolved variables are collected and reported together rather than failing on the first missing variable.

## Validation

Validation runs after parsing in a dedicated `Validate()` method that returns all failures at once.

### Required Fields

| Field | Condition |
|-------|-----------|
| `libraries.movies.root` or `libraries.series.root` | At least one library configured |
| `indexers.prowlarr.url` | If prowlarr section exists |
| `indexers.prowlarr.api_key` | If prowlarr section exists |
| `downloaders.sabnzbd.url` | If sabnzbd section exists |
| `downloaders.sabnzbd.api_key` | If sabnzbd section exists |
| `quality.default` | Must reference a defined profile |

### Value Constraints

| Field | Constraint |
|-------|------------|
| `server.port` | 1-65535 |
| `server.log_level` | One of: debug, info, warn, error |
| `ai.provider` | One of: ollama, anthropic (if ai.enabled) |
| `libraries.*.root` | Directory must exist (warning only) |

### Error Format

```
config validation failed:
  - server.port: must be between 1 and 65535, got 99999
  - quality.default: profile "ultra" not defined
  - indexers.prowlarr.api_key: required when prowlarr is configured
```

## Error Types

```go
type ConfigError struct {
    Path    string   // Config file path (if found)
    Missing []string // Unresolved env vars
    Errors  []string // Validation errors
}
```

## Public API

```go
// Discovery
func Discover() (string, error)           // Find config file
func DefaultPath() string                  // XDG path for new configs

// Loading
func Load(path string) (*Config, error)   // Parse + substitute + validate
func LoadWithoutValidation(path string) (*Config, error)  // For init/debug

// Init support
func WriteDefault(path string) error      // Write example config
func (c *Config) Write(path string) error // Write current config

// Validation
func (c *Config) Validate() error         // Run all validation checks
```

## Load Behavior

1. Parse TOML
2. Substitute env vars, collecting any missing
3. Run validation
4. Return `*Config, error` where error is `*ConfigError` if anything failed

`LoadWithoutValidation()` preserves old behavior for partial configs (generating examples, debugging).
