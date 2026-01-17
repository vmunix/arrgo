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
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoad_MissingEnvVar(t *testing.T) {
	// Use a unique var name that definitely doesn't exist
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[server]
port = 8080

[libraries.movies]
root = "` + tmp + `"

[indexers.prowlarr]
url = "http://localhost"
api_key = "${ARRGO_NONEXISTENT_KEY_12345}"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
	if !strings.Contains(err.Error(), "ARRGO_NONEXISTENT_KEY_12345") {
		t.Errorf("expected ARRGO_NONEXISTENT_KEY_12345 in error, got %v", err)
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
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

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
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

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
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := LoadWithoutValidation(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Port != 99999 {
		t.Errorf("expected port 99999, got %d", cfg.Server.Port)
	}
}

func TestLoad_EnvVarDefault(t *testing.T) {
	// Use a unique var name that doesn't exist to test default syntax
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[server]
host = "${ARRGO_OPTIONAL_VAR_NONEXISTENT:-localhost}"

[libraries.movies]
root = "` + tmp + `"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Server.Host != "localhost" {
		t.Errorf("expected host localhost, got %s", cfg.Server.Host)
	}
}
