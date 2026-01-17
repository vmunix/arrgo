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
