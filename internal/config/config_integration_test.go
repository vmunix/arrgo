package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullWorkflow(t *testing.T) {
	tmp := t.TempDir()

	// 1. Write default config
	cfgPath := filepath.Join(tmp, "arrgo", "config.toml")
	err := WriteDefault(cfgPath)
	require.NoError(t, err, "WriteDefault failed")

	// 2. Set required env vars (t.Setenv auto-restores on cleanup)
	t.Setenv("NZBGEEK_API_KEY", "test-nzbgeek-key")
	t.Setenv("SABNZBD_API_KEY", "test-sab-key")
	t.Setenv("PLEX_TOKEN", "test-plex-token")
	t.Setenv("OVERSEERR_API_KEY", "test-overseerr-key")
	t.Setenv("ARRGO_API_KEY", "test-arrgo-key")
	t.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")

	// 3. Load without validation (library paths don't exist)
	cfg, err := LoadWithoutValidation(cfgPath)
	require.NoError(t, err, "LoadWithoutValidation failed")

	// 4. Verify env substitution worked for indexer
	nzbgeek, ok := cfg.Indexers["nzbgeek"]
	require.True(t, ok, "expected nzbgeek indexer to be configured")
	assert.Equal(t, "test-nzbgeek-key", nzbgeek.APIKey)

	// 5. Verify defaults applied
	assert.Equal(t, 8484, cfg.Server.Port)
}
