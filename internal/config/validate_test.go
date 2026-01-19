// internal/config/validate_test.go
package config

import (
	"os"
	"strings"
	"testing"
)

func TestValidate_MinimalValid(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{
			Movies: LibraryConfig{Root: "/tmp"},
		},
		Indexers: IndexersConfig{
			"nzbgeek": &NewznabConfig{
				URL:    "https://api.nzbgeek.info",
				APIKey: "test-key",
			},
		},
	}
	errs := cfg.Validate()
	if len(errs) != 0 {
		t.Errorf("expected no errors for minimal valid config, got %v", errs)
	}
}

func TestValidate_NoLibrary(t *testing.T) {
	cfg := &Config{}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "at least one library") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected library error, got %v", errs)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := &Config{
		Server:    ServerConfig{Port: 99999},
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "server.port") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected port error, got %v", errs)
	}
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := &Config{
		Server:    ServerConfig{LogLevel: "verbose"},
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "log_level") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected log_level error, got %v", errs)
	}
}

func TestValidate_IndexerMissingAPIKey(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
		Indexers: IndexersConfig{
			"nzbgeek": &NewznabConfig{URL: "https://api.nzbgeek.info"},
		},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "nzbgeek") && strings.Contains(e, "api_key") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected indexer api_key error, got %v", errs)
	}
}

func TestValidate_NoIndexers(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
		Indexers:  IndexersConfig{},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "at least one indexer") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'at least one indexer' error, got %v", errs)
	}
}

func TestValidate_QualityDefaultNotDefined(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
		Quality: QualityConfig{
			Default:  "ultra",
			Profiles: map[string]QualityProfile{"hd": {Resolution: []string{"1080p"}}},
		},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "quality.default") && strings.Contains(e, "ultra") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected quality.default error, got %v", errs)
	}
}

func TestValidate_AIProviderInvalid(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
		AI:        AIConfig{Enabled: true, Provider: "openai"},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "ai.provider") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ai.provider error, got %v", errs)
	}
}

func TestValidate_LibraryRootWarning(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{
			Movies: LibraryConfig{Root: "/nonexistent/path/12345"},
		},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "warning") && strings.Contains(e, "does not exist") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected warning for nonexistent path, got %v", errs)
	}
}

func TestValidate_LibraryRootExists(t *testing.T) {
	tmp := t.TempDir()
	cfg := &Config{
		Libraries: LibrariesConfig{
			Movies: LibraryConfig{Root: tmp},
		},
	}
	errs := cfg.Validate()
	for _, e := range errs {
		if strings.Contains(e, tmp) {
			t.Errorf("unexpected error for existing path: %v", errs)
		}
	}
}

func TestValidate_SABnzbdMissingURL(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: os.TempDir()}},
		Downloaders: DownloadersConfig{
			SABnzbd: &SABnzbdConfig{APIKey: "key"},
		},
	}
	errs := cfg.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e, "sabnzbd") && strings.Contains(e, "url") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sabnzbd url error, got %v", errs)
	}
}
