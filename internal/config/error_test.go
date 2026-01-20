// internal/config/error_test.go
package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError_Error_Empty(t *testing.T) {
	e := &Error{Path: "/etc/arrgo/config.toml"}
	got := e.Error()
	assert.Empty(t, got, "expected empty string for no errors")
}

func TestError_Error_MissingVars(t *testing.T) {
	e := &Error{
		Path:    "/etc/arrgo/config.toml",
		Missing: []string{"API_KEY", "SECRET"},
	}
	got := e.Error()
	assert.Contains(t, got, "missing environment variables")
	assert.Contains(t, got, "API_KEY")
	assert.Contains(t, got, "SECRET")
}

func TestError_Error_ValidationErrors(t *testing.T) {
	e := &Error{
		Path:   "/etc/arrgo/config.toml",
		Errors: []string{"server.port: must be 1-65535", "quality.default: not defined"},
	}
	got := e.Error()
	assert.Contains(t, got, "validation failed")
	assert.Contains(t, got, "server.port")
}

func TestError_Error_Both(t *testing.T) {
	e := &Error{
		Path:    "/etc/arrgo/config.toml",
		Missing: []string{"API_KEY"},
		Errors:  []string{"server.port: invalid"},
	}
	got := e.Error()
	assert.Contains(t, got, "missing environment variables")
	assert.Contains(t, got, "validation failed")
}
