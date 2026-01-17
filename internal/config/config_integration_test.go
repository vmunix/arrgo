package config

import (
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

	// 2. Set required env vars (t.Setenv auto-restores on cleanup)
	t.Setenv("PROWLARR_API_KEY", "test-prowlarr-key")
	t.Setenv("SABNZBD_API_KEY", "test-sab-key")
	t.Setenv("PLEX_TOKEN", "test-plex-token")
	t.Setenv("OVERSEERR_API_KEY", "test-overseerr-key")
	t.Setenv("ARRGO_API_KEY", "test-arrgo-key")
	t.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")

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
