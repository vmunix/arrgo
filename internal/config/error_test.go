// internal/config/error_test.go
package config

import (
	"strings"
	"testing"
)

func TestConfigError_Error_Empty(t *testing.T) {
	e := &ConfigError{Path: "/etc/arrgo/config.toml"}
	got := e.Error()
	if got != "" {
		t.Errorf("expected empty string for no errors, got %q", got)
	}
}

func TestConfigError_Error_MissingVars(t *testing.T) {
	e := &ConfigError{
		Path:    "/etc/arrgo/config.toml",
		Missing: []string{"API_KEY", "SECRET"},
	}
	got := e.Error()
	if !strings.Contains(got, "missing environment variables") {
		t.Errorf("expected 'missing environment variables', got %q", got)
	}
	if !strings.Contains(got, "API_KEY") || !strings.Contains(got, "SECRET") {
		t.Errorf("expected var names in error, got %q", got)
	}
}

func TestConfigError_Error_ValidationErrors(t *testing.T) {
	e := &ConfigError{
		Path:   "/etc/arrgo/config.toml",
		Errors: []string{"server.port: must be 1-65535", "quality.default: not defined"},
	}
	got := e.Error()
	if !strings.Contains(got, "validation failed") {
		t.Errorf("expected 'validation failed', got %q", got)
	}
	if !strings.Contains(got, "server.port") {
		t.Errorf("expected field name in error, got %q", got)
	}
}

func TestConfigError_Error_Both(t *testing.T) {
	e := &ConfigError{
		Path:    "/etc/arrgo/config.toml",
		Missing: []string{"API_KEY"},
		Errors:  []string{"server.port: invalid"},
	}
	got := e.Error()
	if !strings.Contains(got, "missing environment variables") {
		t.Errorf("expected missing vars section, got %q", got)
	}
	if !strings.Contains(got, "validation failed") {
		t.Errorf("expected validation section, got %q", got)
	}
}
