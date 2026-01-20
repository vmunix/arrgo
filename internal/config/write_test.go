// internal/config/write_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteDefault(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "arrgo", "config.toml")

	err := WriteDefault(path)
	require.NoError(t, err, "WriteDefault failed")

	content, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read written file")

	// Check for key sections
	assert.Contains(t, string(content), "[server]")
	assert.Contains(t, string(content), "[libraries.movies]")
	assert.Contains(t, string(content), "${NZBGEEK_API_KEY}")
}

func TestWriteDefault_CreatesDir(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "nested", "deep", "config.toml")

	err := WriteDefault(path)
	require.NoError(t, err, "WriteDefault failed")

	_, err = os.Stat(path)
	assert.False(t, os.IsNotExist(err), "file was not created")
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
	require.NoError(t, err, "Write failed")

	content, _ := os.ReadFile(path)
	assert.Contains(t, string(content), "127.0.0.1")
	assert.Contains(t, string(content), "9000")
}
