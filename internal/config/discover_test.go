package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultPath(t *testing.T) {
	// Clear XDG var to test default
	t.Setenv("XDG_CONFIG_HOME", "")

	path := DefaultPath()
	assert.Contains(t, path, ".config/arrgo/config.toml")
}

func TestDefaultPath_XDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")

	path := DefaultPath()
	assert.Equal(t, "/custom/config/arrgo/config.toml", path)
}

func TestDiscover_ARRGO_CONFIG(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "custom.toml")
	err := os.WriteFile(cfgPath, []byte("[server]"), 0644)
	require.NoError(t, err, "failed to create test config")

	t.Setenv("ARRGO_CONFIG", cfgPath)

	path, err := Discover()
	require.NoError(t, err)
	assert.Equal(t, cfgPath, path)
}

func TestDiscover_ARRGO_CONFIG_NotFound(t *testing.T) {
	t.Setenv("ARRGO_CONFIG", "/nonexistent/config.toml")

	_, err := Discover()
	require.Error(t, err, "expected error for missing ARRGO_CONFIG")
	assert.Contains(t, err.Error(), "ARRGO_CONFIG")
}

func TestDiscover_CurrentDir(t *testing.T) {
	// Save current dir
	origDir, err := os.Getwd()
	require.NoError(t, err, "failed to get working directory")
	defer func() {
		err := os.Chdir(origDir)
		assert.NoError(t, err, "failed to restore working directory")
	}()

	t.Setenv("ARRGO_CONFIG", "")

	// Create temp dir with config
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	err = os.WriteFile(cfgPath, []byte("[server]"), 0644)
	require.NoError(t, err, "failed to create test config")
	err = os.Chdir(tmp)
	require.NoError(t, err, "failed to change directory")

	path, err := Discover()
	require.NoError(t, err)
	assert.True(t, filepath.Base(path) == "config.toml", "expected config.toml, got %s", path)
}

func TestDiscover_NotFound(t *testing.T) {
	// Save current dir
	origDir, err := os.Getwd()
	require.NoError(t, err, "failed to get working directory")
	defer func() {
		err := os.Chdir(origDir)
		assert.NoError(t, err, "failed to restore working directory")
	}()

	t.Setenv("ARRGO_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/nonexistent/xdg")

	tmp := t.TempDir() // Empty temp dir
	err = os.Chdir(tmp)
	require.NoError(t, err, "failed to change directory")

	_, err = Discover()
	require.Error(t, err, "expected error when no config found")
	assert.Contains(t, err.Error(), "config not found")
}
