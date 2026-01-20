package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQualityProfile_AllFields(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[libraries.movies]
root = "` + tmp + `"

[indexers.nzbgeek]
url = "https://api.nzbgeek.info"
api_key = "test-key"

[quality]
default = "premium"

[quality.profiles.premium]
resolution = ["2160p", "1080p"]
sources = ["bluray", "webdl"]
codecs = ["x265", "x264"]
hdr = ["dolby-vision", "hdr10+", "hdr10"]
audio = ["atmos", "truehd", "dtshd"]
prefer_remux = true
reject = ["hdtv", "cam"]
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err, "failed to write test config")

	cfg, err := Load(cfgPath)
	require.NoError(t, err)

	profile, ok := cfg.Quality.Profiles["premium"]
	require.True(t, ok, "expected premium profile to exist")

	// Check resolution
	assert.Equal(t, []string{"2160p", "1080p"}, profile.Resolution)

	// Check sources
	assert.Equal(t, []string{"bluray", "webdl"}, profile.Sources)

	// Check codecs
	assert.Equal(t, []string{"x265", "x264"}, profile.Codecs)

	// Check HDR
	assert.Equal(t, []string{"dolby-vision", "hdr10+", "hdr10"}, profile.HDR)

	// Check audio
	assert.Equal(t, []string{"atmos", "truehd", "dtshd"}, profile.Audio)

	// Check prefer_remux
	assert.True(t, profile.PreferRemux)

	// Check reject
	assert.Equal(t, []string{"hdtv", "cam"}, profile.Reject)
}

func TestQualityProfile_MinimalResolutionOnly(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[libraries.movies]
root = "` + tmp + `"

[indexers.nzbgeek]
url = "https://api.nzbgeek.info"
api_key = "test-key"

[quality]
default = "simple"

[quality.profiles.simple]
resolution = ["1080p"]
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err, "failed to write test config")

	cfg, err := Load(cfgPath)
	require.NoError(t, err)

	profile, ok := cfg.Quality.Profiles["simple"]
	require.True(t, ok, "expected simple profile to exist")

	// Check resolution is set
	assert.Equal(t, []string{"1080p"}, profile.Resolution)

	// All other fields should be nil/empty (no preference)
	assert.Nil(t, profile.Sources)
	assert.Nil(t, profile.Codecs)
	assert.Nil(t, profile.HDR)
	assert.Nil(t, profile.Audio)
	assert.False(t, profile.PreferRemux)
	assert.Nil(t, profile.Reject)
}

func TestQualityProfile_OmittedFieldsNil(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[libraries.movies]
root = "` + tmp + `"

[indexers.nzbgeek]
url = "https://api.nzbgeek.info"
api_key = "test-key"

[quality]
default = "empty"

[quality.profiles.empty]
# No fields specified - all should be nil/zero
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err, "failed to write test config")

	cfg, err := Load(cfgPath)
	require.NoError(t, err)

	profile, ok := cfg.Quality.Profiles["empty"]
	require.True(t, ok, "expected empty profile to exist")

	// All slice fields should be nil (no preference)
	assert.Nil(t, profile.Resolution)
	assert.Nil(t, profile.Sources)
	assert.Nil(t, profile.Codecs)
	assert.Nil(t, profile.HDR)
	assert.Nil(t, profile.Audio)
	assert.False(t, profile.PreferRemux)
	assert.Nil(t, profile.Reject)
}

func TestQualityProfile_PreferenceOrder(t *testing.T) {
	// Verify that array order is preserved (first is best preference)
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	content := `
[libraries.movies]
root = "` + tmp + `"

[indexers.nzbgeek]
url = "https://api.nzbgeek.info"
api_key = "test-key"

[quality.profiles.ordered]
resolution = ["720p", "1080p", "2160p"]
audio = ["aac", "dd", "truehd", "atmos"]
`
	err := os.WriteFile(cfgPath, []byte(content), 0644)
	require.NoError(t, err, "failed to write test config")

	cfg, err := Load(cfgPath)
	require.NoError(t, err)

	profile := cfg.Quality.Profiles["ordered"]

	// Resolution order should be preserved: 720p first (best), then 1080p, then 2160p
	assert.Equal(t, []string{"720p", "1080p", "2160p"}, profile.Resolution)

	// Audio order should be preserved
	assert.Equal(t, []string{"aac", "dd", "truehd", "atmos"}, profile.Audio)
}

// parseTestConfig is a helper that writes content to a temp file and loads it without validation.
func parseTestConfig(t *testing.T, content string) (*Config, error) {
	t.Helper()
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return LoadWithoutValidation(cfgPath)
}

func TestConfig_ImporterCleanupSource(t *testing.T) {
	content := `
[server]
port = 8484

[importer]
cleanup_source = false
`
	cfg, err := parseTestConfig(t, content)
	require.NoError(t, err)

	assert.False(t, cfg.Importer.ShouldCleanupSource(), "CleanupSource should be false when explicitly set")
}

func TestConfig_ImporterCleanupSourceDefault(t *testing.T) {
	content := `
[server]
port = 8484
`
	cfg, err := parseTestConfig(t, content)
	require.NoError(t, err)

	// Default should be true
	assert.True(t, cfg.Importer.ShouldCleanupSource(), "CleanupSource should default to true")
}
