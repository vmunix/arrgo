// internal/config/error.go
package config

import (
	"fmt"
	"strings"
)

// ConfigError aggregates configuration errors.
type ConfigError struct {
	Path    string   // Config file path
	Missing []string // Unresolved environment variables
	Errors  []string // Validation errors
}

func (e *ConfigError) Error() string {
	if len(e.Missing) == 0 && len(e.Errors) == 0 {
		return ""
	}

	var parts []string

	if len(e.Missing) > 0 {
		parts = append(parts, fmt.Sprintf("missing environment variables: %s", strings.Join(e.Missing, ", ")))
	}

	if len(e.Errors) > 0 {
		parts = append(parts, "validation failed:")
		for _, err := range e.Errors {
			parts = append(parts, fmt.Sprintf("  - %s", err))
		}
	}

	return strings.Join(parts, "\n")
}

// HasErrors returns true if there are any errors.
func (e *ConfigError) HasErrors() bool {
	return len(e.Missing) > 0 || len(e.Errors) > 0
}
