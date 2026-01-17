package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPath(t *testing.T) {
	// Clear XDG var to test default
	t.Setenv("XDG_CONFIG_HOME", "")

	path := DefaultPath()
	if !strings.Contains(path, ".config/arrgo/config.toml") {
		t.Errorf("expected ~/.config/arrgo/config.toml, got %s", path)
	}
}

func TestDefaultPath_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")

	path := DefaultPath()
	if path != "/custom/config/arrgo/config.toml" {
		t.Errorf("expected /custom/config/arrgo/config.toml, got %s", path)
	}
}

func TestDiscover_ARRGO_CONFIG(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "custom.toml")
	if err := os.WriteFile(cfgPath, []byte("[server]"), 0644); err != nil {
		t.Fatalf("failed to create test config: %v", err)
	}

	t.Setenv("ARRGO_CONFIG", cfgPath)

	path, err := Discover()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != cfgPath {
		t.Errorf("expected %s, got %s", cfgPath, path)
	}
}

func TestDiscover_ARRGO_CONFIG_NotFound(t *testing.T) {
	t.Setenv("ARRGO_CONFIG", "/nonexistent/config.toml")

	_, err := Discover()
	if err == nil {
		t.Fatal("expected error for missing ARRGO_CONFIG")
	}
	if !strings.Contains(err.Error(), "ARRGO_CONFIG") {
		t.Errorf("expected ARRGO_CONFIG in error, got %v", err)
	}
}

func TestDiscover_CurrentDir(t *testing.T) {
	// Save current dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("failed to restore working directory: %v", err)
		}
	}()

	t.Setenv("ARRGO_CONFIG", "")

	// Create temp dir with config
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[server]"), 0644); err != nil {
		t.Fatalf("failed to create test config: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	path, err := Discover()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(path, "config.toml") {
		t.Errorf("expected config.toml, got %s", path)
	}
}

func TestDiscover_NotFound(t *testing.T) {
	// Save current dir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Errorf("failed to restore working directory: %v", err)
		}
	}()

	t.Setenv("ARRGO_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/nonexistent/xdg")

	tmp := t.TempDir() // Empty temp dir
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	_, err = Discover()
	if err == nil {
		t.Fatal("expected error when no config found")
	}
	if !strings.Contains(err.Error(), "config not found") {
		t.Errorf("expected 'config not found' in error, got %v", err)
	}
}
