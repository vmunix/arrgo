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
