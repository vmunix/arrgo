// internal/config/validate_test.go
package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert.Empty(t, errs, "expected no errors for minimal valid config")
}

func TestValidate_NoLibrary(t *testing.T) {
	cfg := &Config{}
	errs := cfg.Validate()
	assert.True(t, containsError(errs, "at least one library"), "expected library error, got %v", errs)
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := &Config{
		Server:    ServerConfig{Port: 99999},
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
	}
	errs := cfg.Validate()
	assert.True(t, containsError(errs, "server.port"), "expected port error, got %v", errs)
}

func TestValidate_InvalidLogLevel(t *testing.T) {
	cfg := &Config{
		Server:    ServerConfig{LogLevel: "verbose"},
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
	}
	errs := cfg.Validate()
	assert.True(t, containsError(errs, "log_level"), "expected log_level error, got %v", errs)
}

func TestValidate_IndexerMissingAPIKey(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
		Indexers: IndexersConfig{
			"nzbgeek": &NewznabConfig{URL: "https://api.nzbgeek.info"},
		},
	}
	errs := cfg.Validate()
	assert.True(t, containsErrorBoth(errs, "nzbgeek", "api_key"), "expected indexer api_key error, got %v", errs)
}

func TestValidate_NoIndexers(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
		Indexers:  IndexersConfig{},
	}
	errs := cfg.Validate()
	assert.True(t, containsError(errs, "at least one indexer"), "expected 'at least one indexer' error, got %v", errs)
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
	assert.True(t, containsErrorBoth(errs, "quality.default", "ultra"), "expected quality.default error, got %v", errs)
}

func TestValidate_AIProviderInvalid(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: "/tmp"}},
		AI:        AIConfig{Enabled: true, Provider: "openai"},
	}
	errs := cfg.Validate()
	assert.True(t, containsError(errs, "ai.provider"), "expected ai.provider error, got %v", errs)
}

func TestValidate_LibraryRootWarning(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{
			Movies: LibraryConfig{Root: "/nonexistent/path/12345"},
		},
	}
	errs := cfg.Validate()
	assert.True(t, containsErrorBoth(errs, "warning", "does not exist"), "expected warning for nonexistent path, got %v", errs)
}

func TestValidate_LibraryRootExists(t *testing.T) {
	tmp := t.TempDir()
	cfg := &Config{
		Libraries: LibrariesConfig{
			Movies: LibraryConfig{Root: tmp},
		},
	}
	errs := cfg.Validate()
	assert.False(t, containsError(errs, tmp), "unexpected error for existing path: %v", errs)
}

func TestValidate_SABnzbdMissingURL(t *testing.T) {
	cfg := &Config{
		Libraries: LibrariesConfig{Movies: LibraryConfig{Root: os.TempDir()}},
		Downloaders: DownloadersConfig{
			SABnzbd: &SABnzbdConfig{APIKey: "key"},
		},
	}
	errs := cfg.Validate()
	assert.True(t, containsErrorBoth(errs, "sabnzbd", "url"), "expected sabnzbd url error, got %v", errs)
}

// Helper functions to check for errors containing specific strings
func containsError(errs []string, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func containsErrorBoth(errs []string, substr1, substr2 string) bool {
	for _, e := range errs {
		if strings.Contains(e, substr1) && strings.Contains(e, substr2) {
			return true
		}
	}
	return false
}
