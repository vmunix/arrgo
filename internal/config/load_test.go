package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Valid(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[server]
port = 8080

[libraries.movies]
root = "` + tmp + `"

[indexers.nzbgeek]
url = "https://api.nzbgeek.info"
api_key = "test-key"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err, "failed to write test config")

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Server.Port)
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

[indexers.nzbgeek]
url = "http://localhost"
api_key = "${ARRGO_NONEXISTENT_KEY_12345}"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err, "failed to write test config")

	_, err = Load(cfgPath)
	require.Error(t, err, "expected error for missing env var")
	assert.Contains(t, err.Error(), "ARRGO_NONEXISTENT_KEY_12345")
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
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err, "failed to write test config")

	_, err = Load(cfgPath)
	require.Error(t, err, "expected error for invalid port")
	assert.Contains(t, err.Error(), "server.port")
}

func TestLoad_AppliesDefaults(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[libraries.movies]
root = "` + tmp + `"

[indexers.nzbgeek]
url = "https://api.nzbgeek.info"
api_key = "test-key"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err, "failed to write test config")

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8484, cfg.Server.Port)
}

func TestLoadWithoutValidation(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[server]
port = 99999
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err, "failed to write test config")

	cfg, err := LoadWithoutValidation(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 99999, cfg.Server.Port)
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

[indexers.nzbgeek]
url = "https://api.nzbgeek.info"
api_key = "test-key"
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err, "failed to write test config")

	cfg, err := Load(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "localhost", cfg.Server.Host)
}
