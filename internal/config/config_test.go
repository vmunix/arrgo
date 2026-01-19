package config

import (
	"os"
	"path/filepath"
	"testing"
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
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	profile, ok := cfg.Quality.Profiles["premium"]
	if !ok {
		t.Fatal("expected premium profile to exist")
	}

	// Check resolution
	expectedRes := []string{"2160p", "1080p"}
	if len(profile.Resolution) != len(expectedRes) {
		t.Errorf("resolution: expected %v, got %v", expectedRes, profile.Resolution)
	}
	for i, v := range expectedRes {
		if profile.Resolution[i] != v {
			t.Errorf("resolution[%d]: expected %q, got %q", i, v, profile.Resolution[i])
		}
	}

	// Check sources
	expectedSources := []string{"bluray", "webdl"}
	if len(profile.Sources) != len(expectedSources) {
		t.Errorf("sources: expected %v, got %v", expectedSources, profile.Sources)
	}
	for i, v := range expectedSources {
		if profile.Sources[i] != v {
			t.Errorf("sources[%d]: expected %q, got %q", i, v, profile.Sources[i])
		}
	}

	// Check codecs
	expectedCodecs := []string{"x265", "x264"}
	if len(profile.Codecs) != len(expectedCodecs) {
		t.Errorf("codecs: expected %v, got %v", expectedCodecs, profile.Codecs)
	}
	for i, v := range expectedCodecs {
		if profile.Codecs[i] != v {
			t.Errorf("codecs[%d]: expected %q, got %q", i, v, profile.Codecs[i])
		}
	}

	// Check HDR
	expectedHDR := []string{"dolby-vision", "hdr10+", "hdr10"}
	if len(profile.HDR) != len(expectedHDR) {
		t.Errorf("hdr: expected %v, got %v", expectedHDR, profile.HDR)
	}
	for i, v := range expectedHDR {
		if profile.HDR[i] != v {
			t.Errorf("hdr[%d]: expected %q, got %q", i, v, profile.HDR[i])
		}
	}

	// Check audio
	expectedAudio := []string{"atmos", "truehd", "dtshd"}
	if len(profile.Audio) != len(expectedAudio) {
		t.Errorf("audio: expected %v, got %v", expectedAudio, profile.Audio)
	}
	for i, v := range expectedAudio {
		if profile.Audio[i] != v {
			t.Errorf("audio[%d]: expected %q, got %q", i, v, profile.Audio[i])
		}
	}

	// Check prefer_remux
	if !profile.PreferRemux {
		t.Error("prefer_remux: expected true, got false")
	}

	// Check reject
	expectedReject := []string{"hdtv", "cam"}
	if len(profile.Reject) != len(expectedReject) {
		t.Errorf("reject: expected %v, got %v", expectedReject, profile.Reject)
	}
	for i, v := range expectedReject {
		if profile.Reject[i] != v {
			t.Errorf("reject[%d]: expected %q, got %q", i, v, profile.Reject[i])
		}
	}
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
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	profile, ok := cfg.Quality.Profiles["simple"]
	if !ok {
		t.Fatal("expected simple profile to exist")
	}

	// Check resolution is set
	if len(profile.Resolution) != 1 || profile.Resolution[0] != "1080p" {
		t.Errorf("resolution: expected [1080p], got %v", profile.Resolution)
	}

	// All other fields should be nil/empty (no preference)
	if profile.Sources != nil {
		t.Errorf("sources: expected nil, got %v", profile.Sources)
	}
	if profile.Codecs != nil {
		t.Errorf("codecs: expected nil, got %v", profile.Codecs)
	}
	if profile.HDR != nil {
		t.Errorf("hdr: expected nil, got %v", profile.HDR)
	}
	if profile.Audio != nil {
		t.Errorf("audio: expected nil, got %v", profile.Audio)
	}
	if profile.PreferRemux {
		t.Error("prefer_remux: expected false, got true")
	}
	if profile.Reject != nil {
		t.Errorf("reject: expected nil, got %v", profile.Reject)
	}
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
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	profile, ok := cfg.Quality.Profiles["empty"]
	if !ok {
		t.Fatal("expected empty profile to exist")
	}

	// All slice fields should be nil (no preference)
	if profile.Resolution != nil {
		t.Errorf("resolution: expected nil, got %v", profile.Resolution)
	}
	if profile.Sources != nil {
		t.Errorf("sources: expected nil, got %v", profile.Sources)
	}
	if profile.Codecs != nil {
		t.Errorf("codecs: expected nil, got %v", profile.Codecs)
	}
	if profile.HDR != nil {
		t.Errorf("hdr: expected nil, got %v", profile.HDR)
	}
	if profile.Audio != nil {
		t.Errorf("audio: expected nil, got %v", profile.Audio)
	}
	if profile.PreferRemux {
		t.Error("prefer_remux: expected false (default), got true")
	}
	if profile.Reject != nil {
		t.Errorf("reject: expected nil, got %v", profile.Reject)
	}
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
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	profile := cfg.Quality.Profiles["ordered"]

	// Resolution order should be preserved: 720p first (best), then 1080p, then 2160p
	expectedRes := []string{"720p", "1080p", "2160p"}
	for i, v := range expectedRes {
		if profile.Resolution[i] != v {
			t.Errorf("resolution order not preserved at index %d: expected %q, got %q", i, v, profile.Resolution[i])
		}
	}

	// Audio order should be preserved
	expectedAudio := []string{"aac", "dd", "truehd", "atmos"}
	for i, v := range expectedAudio {
		if profile.Audio[i] != v {
			t.Errorf("audio order not preserved at index %d: expected %q, got %q", i, v, profile.Audio[i])
		}
	}
}
