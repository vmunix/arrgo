// internal/config/discover.go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPath returns the XDG-compliant default config path.
func DefaultPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "./config.toml"
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "arrgo", "config.toml")
}

// Discover finds the config file using the standard search order.
// Search order:
//  1. ARRGO_CONFIG environment variable
//  2. ./config.toml (current directory)
//  3. $XDG_CONFIG_HOME/arrgo/config.toml
//  4. /etc/arrgo/config.toml
func Discover() (string, error) {
	// 1. Check ARRGO_CONFIG env var
	if envPath := os.Getenv("ARRGO_CONFIG"); envPath != "" {
		if _, err := os.Stat(envPath); err != nil {
			return "", fmt.Errorf("ARRGO_CONFIG=%s: %w", envPath, err)
		}
		return envPath, nil
	}

	// Build search paths
	paths := []string{
		"./config.toml",
		DefaultPath(),
		"/etc/arrgo/config.toml",
	}

	// 2-4. Check each path
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("config not found, checked: %s", formatPaths(paths))
}

func formatPaths(paths []string) string {
	return strings.Join(paths, ", ")
}
