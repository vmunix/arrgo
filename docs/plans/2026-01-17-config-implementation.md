# Config Module Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the config module production-ready with XDG discovery, enhanced env var substitution, and validation.

**Architecture:** Extend existing `internal/config/config.go` with new error type, discovery functions, enhanced substitution, and validation. Keep backward compatibility via `LoadWithoutValidation()`.

**Tech Stack:** Go stdlib, `github.com/BurntSushi/toml`, `os` for XDG paths.

---

### Task 1: ConfigError Type

**Files:**
- Create: `internal/config/error.go`
- Create: `internal/config/error_test.go`

**Step 1: Write the failing test**

```go
// internal/config/error_test.go
package config

import (
	"strings"
	"testing"
)

func TestConfigError_Error_Empty(t *testing.T) {
	e := &ConfigError{Path: "/etc/arrgo/config.toml"}
	got := e.Error()
	if got != "" {
		t.Errorf("expected empty string for no errors, got %q", got)
	}
}

func TestConfigError_Error_MissingVars(t *testing.T) {
	e := &ConfigError{
		Path:    "/etc/arrgo/config.toml",
		Missing: []string{"API_KEY", "SECRET"},
	}
	got := e.Error()
	if !strings.Contains(got, "missing environment variables") {
		t.Errorf("expected 'missing environment variables', got %q", got)
	}
	if !strings.Contains(got, "API_KEY") || !strings.Contains(got, "SECRET") {
		t.Errorf("expected var names in error, got %q", got)
	}
}

func TestConfigError_Error_ValidationErrors(t *testing.T) {
	e := &ConfigError{
		Path:   "/etc/arrgo/config.toml",
		Errors: []string{"server.port: must be 1-65535", "quality.default: not defined"},
	}
	got := e.Error()
	if !strings.Contains(got, "validation failed") {
		t.Errorf("expected 'validation failed', got %q", got)
	}
	if !strings.Contains(got, "server.port") {
		t.Errorf("expected field name in error, got %q", got)
	}
}

func TestConfigError_Error_Both(t *testing.T) {
	e := &ConfigError{
		Path:    "/etc/arrgo/config.toml",
		Missing: []string{"API_KEY"},
		Errors:  []string{"server.port: invalid"},
	}
	got := e.Error()
	if !strings.Contains(got, "missing environment variables") {
		t.Errorf("expected missing vars section, got %q", got)
	}
	if !strings.Contains(got, "validation failed") {
		t.Errorf("expected validation section, got %q", got)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run TestConfigError -v`
Expected: FAIL - file does not exist

**Step 3: Write implementation**

```go
// internal/config/error.go
package config

import (
	"fmt"
	"strings"
)

// ConfigError aggregates configuration errors.
type ConfigError struct {
	Path    string   // Config file path
	Missing []string // Unresolved environment variables
	Errors  []string // Validation errors
}

func (e *ConfigError) Error() string {
	if len(e.Missing) == 0 && len(e.Errors) == 0 {
		return ""
	}

	var parts []string

	if len(e.Missing) > 0 {
		parts = append(parts, fmt.Sprintf("missing environment variables: %s", strings.Join(e.Missing, ", ")))
	}

	if len(e.Errors) > 0 {
		parts = append(parts, "validation failed:")
		for _, err := range e.Errors {
			parts = append(parts, fmt.Sprintf("  - %s", err))
		}
	}

	return strings.Join(parts, "\n")
}

// HasErrors returns true if there are any errors.
func (e *ConfigError) HasErrors() bool {
	return len(e.Missing) > 0 || len(e.Errors) > 0
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config -run TestConfigError -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/error.go internal/config/error_test.go
git commit -m "feat(config): add ConfigError type for aggregated errors"
```

---

### Task 2: Enhanced Environment Variable Substitution

**Files:**
- Modify: `internal/config/config.go:154-165`
- Create: `internal/config/envsubst_test.go`

**Step 1: Write the failing tests**

```go
// internal/config/envsubst_test.go
package config

import (
	"os"
	"testing"
)

func TestSubstituteEnvVars_Simple(t *testing.T) {
	os.Setenv("TEST_VAR", "hello")
	defer os.Unsetenv("TEST_VAR")

	content, missing := substituteEnvVars("value = ${TEST_VAR}")
	if content != "value = hello" {
		t.Errorf("expected 'value = hello', got %q", content)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing vars, got %v", missing)
	}
}

func TestSubstituteEnvVars_Missing(t *testing.T) {
	os.Unsetenv("MISSING_VAR")

	content, missing := substituteEnvVars("value = ${MISSING_VAR}")
	if content != "value = ${MISSING_VAR}" {
		t.Errorf("expected unchanged, got %q", content)
	}
	if len(missing) != 1 || missing[0] != "MISSING_VAR" {
		t.Errorf("expected [MISSING_VAR], got %v", missing)
	}
}

func TestSubstituteEnvVars_Default(t *testing.T) {
	os.Unsetenv("UNSET_VAR")

	content, missing := substituteEnvVars("value = ${UNSET_VAR:-default_value}")
	if content != "value = default_value" {
		t.Errorf("expected 'value = default_value', got %q", content)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing vars with default, got %v", missing)
	}
}

func TestSubstituteEnvVars_DefaultOverriddenByEnv(t *testing.T) {
	os.Setenv("SET_VAR", "from_env")
	defer os.Unsetenv("SET_VAR")

	content, missing := substituteEnvVars("value = ${SET_VAR:-default}")
	if content != "value = from_env" {
		t.Errorf("expected 'value = from_env', got %q", content)
	}
	if len(missing) != 0 {
		t.Errorf("expected no missing vars, got %v", missing)
	}
}

func TestSubstituteEnvVars_RequiredError(t *testing.T) {
	os.Unsetenv("REQUIRED_VAR")

	content, missing := substituteEnvVars("value = ${REQUIRED_VAR:?API key is required}")
	if content != "value = ${REQUIRED_VAR:?API key is required}" {
		t.Errorf("expected unchanged, got %q", content)
	}
	if len(missing) != 1 || missing[0] != "REQUIRED_VAR: API key is required" {
		t.Errorf("expected error message, got %v", missing)
	}
}

func TestSubstituteEnvVars_Multiple(t *testing.T) {
	os.Setenv("VAR1", "one")
	os.Unsetenv("VAR2")
	os.Unsetenv("VAR3")
	defer os.Unsetenv("VAR1")

	content, missing := substituteEnvVars("${VAR1} ${VAR2} ${VAR3:-three}")
	if content != "one ${VAR2} three" {
		t.Errorf("expected 'one ${VAR2} three', got %q", content)
	}
	if len(missing) != 1 || missing[0] != "VAR2" {
		t.Errorf("expected [VAR2], got %v", missing)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run TestSubstituteEnvVars -v`
Expected: FAIL - wrong function signature

**Step 3: Update implementation**

Replace the existing `substituteEnvVars` function in `internal/config/config.go`:

```go
// substituteEnvVars replaces ${VAR}, ${VAR:-default}, ${VAR:?error} patterns.
// Returns the substituted content and a list of missing/error variables.
var envVarPattern = regexp.MustCompile(`\$\{([^}:]+)(?:(:[-?])([^}]*))?\}`)

func substituteEnvVars(content string) (string, []string) {
	var missing []string

	result := envVarPattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := envVarPattern.FindStringSubmatch(match)
		varName := parts[1]
		modifier := parts[2]
		modValue := parts[3]

		value, exists := os.LookupEnv(varName)

		switch modifier {
		case ":-": // Default value
			if !exists || value == "" {
				return modValue
			}
			return value
		case ":?": // Required with error
			if !exists || value == "" {
				missing = append(missing, varName+": "+modValue)
				return match
			}
			return value
		default: // Simple substitution
			if exists {
				return value
			}
			missing = append(missing, varName)
			return match
		}
	})

	return result, missing
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config -run TestSubstituteEnvVars -v`
Expected: PASS

**Step 5: Update Load function to handle missing vars**

The existing `Load` function calls `substituteEnvVars` but ignores the second return value. We'll fix this in Task 6 when we wire everything together.

**Step 6: Commit**

```bash
git add internal/config/config.go internal/config/envsubst_test.go
git commit -m "feat(config): enhanced env var substitution with defaults and required syntax"
```

---

### Task 3: XDG Config Discovery

**Files:**
- Create: `internal/config/discover.go`
- Create: `internal/config/discover_test.go`

**Step 1: Write the failing tests**

```go
// internal/config/discover_test.go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPath(t *testing.T) {
	// Clear XDG var to test default
	old := os.Getenv("XDG_CONFIG_HOME")
	os.Unsetenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", old)

	path := DefaultPath()
	if !strings.Contains(path, ".config/arrgo/config.toml") {
		t.Errorf("expected ~/.config/arrgo/config.toml, got %s", path)
	}
}

func TestDefaultPath_XDG(t *testing.T) {
	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	defer os.Unsetenv("XDG_CONFIG_HOME")

	path := DefaultPath()
	if path != "/custom/config/arrgo/config.toml" {
		t.Errorf("expected /custom/config/arrgo/config.toml, got %s", path)
	}
}

func TestDiscover_ARRGO_CONFIG(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "custom.toml")
	os.WriteFile(cfgPath, []byte("[server]"), 0644)

	os.Setenv("ARRGO_CONFIG", cfgPath)
	defer os.Unsetenv("ARRGO_CONFIG")

	path, err := Discover()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != cfgPath {
		t.Errorf("expected %s, got %s", cfgPath, path)
	}
}

func TestDiscover_ARRGO_CONFIG_NotFound(t *testing.T) {
	os.Setenv("ARRGO_CONFIG", "/nonexistent/config.toml")
	defer os.Unsetenv("ARRGO_CONFIG")

	_, err := Discover()
	if err == nil {
		t.Fatal("expected error for missing ARRGO_CONFIG")
	}
	if !strings.Contains(err.Error(), "ARRGO_CONFIG") {
		t.Errorf("expected ARRGO_CONFIG in error, got %v", err)
	}
}

func TestDiscover_CurrentDir(t *testing.T) {
	// Save current dir and env
	origDir, _ := os.Getwd()
	origEnv := os.Getenv("ARRGO_CONFIG")
	os.Unsetenv("ARRGO_CONFIG")
	defer func() {
		os.Chdir(origDir)
		if origEnv != "" {
			os.Setenv("ARRGO_CONFIG", origEnv)
		}
	}()

	// Create temp dir with config
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	os.WriteFile(cfgPath, []byte("[server]"), 0644)
	os.Chdir(tmp)

	path, err := Discover()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(path, "config.toml") {
		t.Errorf("expected config.toml, got %s", path)
	}
}

func TestDiscover_NotFound(t *testing.T) {
	// Clear all discovery paths
	os.Unsetenv("ARRGO_CONFIG")
	origXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", "/nonexistent/xdg")
	defer os.Setenv("XDG_CONFIG_HOME", origXDG)

	origDir, _ := os.Getwd()
	tmp := t.TempDir() // Empty temp dir
	os.Chdir(tmp)
	defer os.Chdir(origDir)

	_, err := Discover()
	if err == nil {
		t.Fatal("expected error when no config found")
	}
	if !strings.Contains(err.Error(), "config not found") {
		t.Errorf("expected 'config not found' in error, got %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run TestDiscover -v && go test ./internal/config -run TestDefaultPath -v`
Expected: FAIL - functions don't exist

**Step 3: Write implementation**

```go
// internal/config/discover.go
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultPath returns the XDG-compliant default config path.
func DefaultPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "./config.toml"
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "arrgo", "config.toml")
}

// Discover finds the config file using the standard search order.
// Search order:
//  1. ARRGO_CONFIG environment variable
//  2. ./config.toml (current directory)
//  3. $XDG_CONFIG_HOME/arrgo/config.toml
//  4. /etc/arrgo/config.toml
func Discover() (string, error) {
	// 1. Check ARRGO_CONFIG env var
	if envPath := os.Getenv("ARRGO_CONFIG"); envPath != "" {
		if _, err := os.Stat(envPath); err != nil {
			return "", fmt.Errorf("ARRGO_CONFIG=%s: %w", envPath, err)
		}
		return envPath, nil
	}

	// Build search paths
	paths := []string{
		"./config.toml",
		DefaultPath(),
		"/etc/arrgo/config.toml",
	}

	// 2-4. Check each path
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("config not found, checked: %s", formatPaths(paths))
}

func formatPaths(paths []string) string {
	result := ""
	for i, p := range paths {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config -run "TestDiscover|TestDefaultPath" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/discover.go internal/config/discover_test.go
git commit -m "feat(config): add XDG-aware config discovery"
```

---

### Task 4: Validation

**Files:**
- Create: `internal/config/validate.go`
- Create: `internal/config/validate_test.go`

**Step 1: Write the failing tests**

```go
// internal/config/validate_test.go
package config

import (
	"os"
	"strings"
	"testing"
)

func TestValidate_MinimalValid(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{
			Movies: LibraryConfig{Root: "/tmp"},
		},
	}
	errs := cfg.Validate()
	if len(errs) != 0 {
		t.Errorf("expected no errors for minimal valid config, got %v", errs)
	}
}

func TestValidate_NoLibrary(t *testing.T) {
	cfg := &Config{}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "at least one library") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected library error, got %v", errs)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := &Config{
		Server:    ServerConfig{Port: 99999},
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "server.port") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected port error, got %v", errs)
	}
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := &Config{
		Server:    ServerConfig{LogLevel: "verbose"},
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "log_level") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected log_level error, got %v", errs)
	}
}

func TestValidate_ProwlarrMissingAPIKey(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
		Indexers: IndexersConfig{
			Prowlarr: &ProwlarrConfig{URL: "http://localhost:9696"},
		},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "prowlarr") && strings.Contains(e, "api_key") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected prowlarr api_key error, got %v", errs)
	}
}

func TestValidate_QualityDefaultNotDefined(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
		Quality: QualityConfig{
			Default:  "ultra",
			Profiles: map[string]QualityProfile{"hd": {Accept: []string{"1080p"}}},
		},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "quality.default") && strings.Contains(e, "ultra") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected quality.default error, got %v", errs)
	}
}

func TestValidate_AIProviderInvalid(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
		AI:        AIConfig{Enabled: true, Provider: "openai"},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "ai.provider") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ai.provider error, got %v", errs)
	}
}

func TestValidate_LibraryRootWarning(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{
			Movies: LibraryConfig{Root: "/nonexistent/path/12345"},
		},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "warning") && strings.Contains(e, "does not exist") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning for nonexistent path, got %v", errs)
	}
}

func TestValidate_LibraryRootExists(t *testing.T) {
	tmp := t.TempDir()
	cfg := &Config{
		Libraries: LibrariesConfig{
			Movies: LibraryConfig{Root: tmp},
		},
	}
	errs := cfg.Validate()
	for _, e := range errs {
		if strings.Contains(e, tmp) {
			t.Errorf("unexpected error for existing path: %v", errs)
		}
	}
}

func TestValidate_SABnzbdMissingURL(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: os.TempDir()}},
		Downloaders: DownloadersConfig{
			SABnzbd: &SABnzbdConfig{APIKey: "key"},
		},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "sabnzbd") && strings.Contains(e, "url") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sabnzbd url error, got %v", errs)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run TestValidate -v`
Expected: FAIL - Validate method doesn't exist

**Step 3: Write implementation**

```go
// internal/config/validate.go
package config

import (
	"fmt"
	"os"
)

var validLogLevels = map[string]bool{
	"debug": true, "info": true, "warn": true, "error": true, "": true,
}

var validAIProviders = map[string]bool{
	"ollama": true, "anthropic": true,
}

// Validate checks the configuration for errors.
// Returns a slice of error messages (empty if valid).
func (c *Config) Validate() []string {
	var errs []string

	// At least one library required
	if c.Libraries.Movies.Root == "" && c.Libraries.Series.Root == "" {
		errs = append(errs, "libraries: at least one library (movies or series) must be configured")
	}

	// Server validation
	if c.Server.Port != 0 && (c.Server.Port < 1 || c.Server.Port > 65535) {
		errs = append(errs, fmt.Sprintf("server.port: must be between 1 and 65535, got %d", c.Server.Port))
	}
	if !validLogLevels[c.Server.LogLevel] {
		errs = append(errs, fmt.Sprintf("server.log_level: must be one of debug, info, warn, error; got %q", c.Server.LogLevel))
	}

	// Quality validation
	if c.Quality.Default != "" && len(c.Quality.Profiles) > 0 {
		if _, ok := c.Quality.Profiles[c.Quality.Default]; !ok {
			errs = append(errs, fmt.Sprintf("quality.default: profile %q not defined", c.Quality.Default))
		}
	}

	// Prowlarr validation
	if c.Indexers.Prowlarr != nil {
		if c.Indexers.Prowlarr.URL == "" {
			errs = append(errs, "indexers.prowlarr.url: required when prowlarr is configured")
		}
		if c.Indexers.Prowlarr.APIKey == "" {
			errs = append(errs, "indexers.prowlarr.api_key: required when prowlarr is configured")
		}
	}

	// SABnzbd validation
	if c.Downloaders.SABnzbd != nil {
		if c.Downloaders.SABnzbd.URL == "" {
			errs = append(errs, "downloaders.sabnzbd.url: required when sabnzbd is configured")
		}
		if c.Downloaders.SABnzbd.APIKey == "" {
			errs = append(errs, "downloaders.sabnzbd.api_key: required when sabnzbd is configured")
		}
	}

	// AI validation
	if c.AI.Enabled {
		if !validAIProviders[c.AI.Provider] {
			errs = append(errs, fmt.Sprintf("ai.provider: must be one of ollama, anthropic; got %q", c.AI.Provider))
		}
	}

	// Library path warnings (non-fatal)
	if c.Libraries.Movies.Root != "" {
		if _, err := os.Stat(c.Libraries.Movies.Root); os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("libraries.movies.root: warning: directory %q does not exist", c.Libraries.Movies.Root))
		}
	}
	if c.Libraries.Series.Root != "" {
		if _, err := os.Stat(c.Libraries.Series.Root); os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("libraries.series.root: warning: directory %q does not exist", c.Libraries.Series.Root))
		}
	}

	return errs
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config -run TestValidate -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/validate.go internal/config/validate_test.go
git commit -m "feat(config): add validation with required fields and value constraints"
```

---

### Task 5: Init Support (WriteDefault, Write)

**Files:**
- Create: `internal/config/write.go`
- Create: `internal/config/write_test.go`

**Step 1: Write the failing tests**

```go
// internal/config/write_test.go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteDefault(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "arrgo", "config.toml")

	err := WriteDefault(path)
	if err != nil {
		t.Fatalf("WriteDefault failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	// Check for key sections
	if !strings.Contains(string(content), "[server]") {
		t.Error("expected [server] section")
	}
	if !strings.Contains(string(content), "[libraries.movies]") {
		t.Error("expected [libraries.movies] section")
	}
	if !strings.Contains(string(content), "${PROWLARR_API_KEY}") {
		t.Error("expected env var placeholder")
	}
}

func TestWriteDefault_CreatesDir(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nested", "deep", "config.toml")

	err := WriteDefault(path)
	if err != nil {
		t.Fatalf("WriteDefault failed: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("file was not created")
	}
}

func TestConfig_Write(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{Host: "127.0.0.1", Port: 9000},
		Libraries: LibrariesConfig{
			Movies: LibraryConfig{Root: "/media/movies"},
		},
	}

	tmp := t.TempDir()
	path := filepath.Join(tmp, "config.toml")

	err := cfg.Write(path)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "127.0.0.1") {
		t.Error("expected host in output")
	}
	if !strings.Contains(string(content), "9000") {
		t.Error("expected port in output")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run "TestWriteDefault|TestConfig_Write" -v`
Expected: FAIL - functions don't exist

**Step 3: Write implementation**

```go
// internal/config/write.go
package config

import (
	_ "embed"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

//go:embed default_config.toml
var defaultConfig string

// WriteDefault writes the example config to the specified path.
// Creates parent directories if needed.
func WriteDefault(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultConfig), 0644)
}

// Write serializes the config to TOML and writes it to the specified path.
func (c *Config) Write(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	return encoder.Encode(c)
}
```

**Step 4: Create embedded default config**

Copy `config.example.toml` to `internal/config/default_config.toml`:

```bash
cp config.example.toml internal/config/default_config.toml
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/config -run "TestWriteDefault|TestConfig_Write" -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/config/write.go internal/config/write_test.go internal/config/default_config.toml
git commit -m "feat(config): add WriteDefault and Write for config init"
```

---

### Task 6: Wire Everything Together in Load

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/load_test.go`

**Step 1: Write the failing tests**

```go
// internal/config/load_test.go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_Valid(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[server]
port = 8080

[libraries.movies]
root = "` + tmp + `"
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoad_MissingEnvVar(t *testing.T) {
	os.Unsetenv("MISSING_KEY")
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[server]
port = 8080

[libraries.movies]
root = "` + tmp + `"

[indexers.prowlarr]
url = "http://localhost"
api_key = "${MISSING_KEY}"
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
	if !strings.Contains(err.Error(), "MISSING_KEY") {
		t.Errorf("expected MISSING_KEY in error, got %v", err)
	}
}

func TestLoad_ValidationError(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[server]
port = 99999

[libraries.movies]
root = "` + tmp + `"
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
	if !strings.Contains(err.Error(), "server.port") {
		t.Errorf("expected server.port in error, got %v", err)
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[libraries.movies]
root = "` + tmp + `"
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8484 {
		t.Errorf("expected default port 8484, got %d", cfg.Server.Port)
	}
}

func TestLoadWithoutValidation(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[server]
port = 99999
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := LoadWithoutValidation(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 99999 {
		t.Errorf("expected port 99999, got %d", cfg.Server.Port)
	}
}

func TestLoad_EnvVarDefault(t *testing.T) {
	os.Unsetenv("OPTIONAL_VAR")
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[server]
host = "${OPTIONAL_VAR:-localhost}"

[libraries.movies]
root = "` + tmp + `"
`
	os.WriteFile(cfgPath, []byte(content), 0644)

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "localhost" {
		t.Errorf("expected host localhost, got %s", cfg.Server.Host)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run "TestLoad" -v`
Expected: FAIL - Load signature changed, LoadWithoutValidation missing

**Step 3: Update Load and add LoadWithoutValidation**

Replace the `Load` function in `internal/config/config.go`:

```go
// Load reads, parses, and validates the configuration file.
func Load(path string) (*Config, error) {
	cfg, missing, err := load(path)
	if err != nil {
		return nil, err
	}

	// Build ConfigError if any issues
	configErr := &ConfigError{Path: path, Missing: missing}

	// Run validation
	configErr.Errors = cfg.Validate()

	if configErr.HasErrors() {
		return nil, configErr
	}

	return cfg, nil
}

// LoadWithoutValidation reads and parses the config without validation.
// Useful for init commands or debugging.
func LoadWithoutValidation(path string) (*Config, error) {
	cfg, _, err := load(path)
	return cfg, err
}

// load is the internal loader that returns config, missing vars, and parse error.
func load(path string) (*Config, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading config: %w", err)
	}

	// Substitute environment variables
	content, missing := substituteEnvVars(string(data))

	var cfg Config
	if _, err := toml.Decode(content, &cfg); err != nil {
		return nil, nil, fmt.Errorf("parsing config: %w", err)
	}

	// Apply defaults
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8484
	}
	if cfg.Server.LogLevel == "" {
		cfg.Server.LogLevel = "info"
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "./data/arrgo.db"
	}

	return &cfg, missing, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config -run "TestLoad" -v`
Expected: PASS

**Step 5: Run all tests to verify nothing broke**

Run: `go test ./internal/config -v`
Expected: All PASS

**Step 6: Commit**

```bash
git add internal/config/config.go internal/config/load_test.go
git commit -m "feat(config): wire up Load with env substitution and validation"
```

---

### Task 7: Final Integration Test

**Files:**
- Create: `internal/config/config_integration_test.go`

**Step 1: Write integration test**

```go
// internal/config/config_integration_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFullWorkflow(t *testing.T) {
	tmp := t.TempDir()

	// 1. Write default config
	cfgPath := filepath.Join(tmp, "arrgo", "config.toml")
	if err := WriteDefault(cfgPath); err != nil {
		t.Fatalf("WriteDefault: %v", err)
	}

	// 2. Set required env vars
	os.Setenv("PROWLARR_API_KEY", "test-prowlarr-key")
	os.Setenv("SABNZBD_API_KEY", "test-sab-key")
	os.Setenv("PLEX_TOKEN", "test-plex-token")
	os.Setenv("OVERSEERR_API_KEY", "test-overseerr-key")
	os.Setenv("ARRGO_API_KEY", "test-arrgo-key")
	os.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	defer func() {
		os.Unsetenv("PROWLARR_API_KEY")
		os.Unsetenv("SABNZBD_API_KEY")
		os.Unsetenv("PLEX_TOKEN")
		os.Unsetenv("OVERSEERR_API_KEY")
		os.Unsetenv("ARRGO_API_KEY")
		os.Unsetenv("ANTHROPIC_API_KEY")
	}()

	// 3. Load without validation (library paths don't exist)
	cfg, err := LoadWithoutValidation(cfgPath)
	if err != nil {
		t.Fatalf("LoadWithoutValidation: %v", err)
	}

	// 4. Verify env substitution worked
	if cfg.Indexers.Prowlarr.APIKey != "test-prowlarr-key" {
		t.Errorf("expected prowlarr key substituted, got %q", cfg.Indexers.Prowlarr.APIKey)
	}

	// 5. Verify defaults applied
	if cfg.Server.Port != 8484 {
		t.Errorf("expected default port 8484, got %d", cfg.Server.Port)
	}
}
```

**Step 2: Run integration test**

Run: `go test ./internal/config -run TestFullWorkflow -v`
Expected: PASS

**Step 3: Run all config tests**

Run: `go test ./internal/config -v`
Expected: All PASS

**Step 4: Commit**

```bash
git add internal/config/config_integration_test.go
git commit -m "test(config): add full workflow integration test"
```

---

### Task 8: Build Verification

**Step 1: Build the project**

Run: `go build ./cmd/arrgo`
Expected: Build succeeds with no errors

**Step 2: Run all tests**

Run: `go test ./...`
Expected: All PASS

**Step 3: Final commit (if any cleanup needed)**

```bash
git status
```

If clean, done. Otherwise commit any final changes.

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | ConfigError type | error.go, error_test.go |
| 2 | Enhanced env substitution | config.go, envsubst_test.go |
| 3 | XDG config discovery | discover.go, discover_test.go |
| 4 | Validation | validate.go, validate_test.go |
| 5 | Init support | write.go, write_test.go, default_config.toml |
| 6 | Wire up Load | config.go, load_test.go |
| 7 | Integration test | config_integration_test.go |
| 8 | Build verification | - |
