// internal/config/validate.go
package config

import (
	"fmt"
	"os"
)

var validLogLevels = map[string]bool{
	"debug": true, "info": true, "warn": true, "error": true, "": true,
}

var validAIProviders = map[string]bool{
	"ollama": true, "anthropic": true,
}

// Validate checks the configuration for errors.
// Returns a slice of error messages (empty if valid).
func (c *Config) Validate() []string {
	var errs []string

	// At least one library required
	if c.Libraries.Movies.Root == "" && c.Libraries.Series.Root == "" {
		errs = append(errs, "libraries: at least one library (movies or series) must be configured")
	}

	// Server validation
	if c.Server.Port != 0 && (c.Server.Port < 1 || c.Server.Port > 65535) {
		errs = append(errs, fmt.Sprintf("server.port: must be between 1 and 65535, got %d", c.Server.Port))
	}
	if !validLogLevels[c.Server.LogLevel] {
		errs = append(errs, fmt.Sprintf("server.log_level: must be one of debug, info, warn, error; got %q", c.Server.LogLevel))
	}

	// Quality validation
	if c.Quality.Default != "" && len(c.Quality.Profiles) > 0 {
		if _, ok := c.Quality.Profiles[c.Quality.Default]; !ok {
			errs = append(errs, fmt.Sprintf("quality.default: profile %q not defined", c.Quality.Default))
		}
	}

	// Indexers validation
	if len(c.Indexers) == 0 {
		errs = append(errs, "indexers: at least one indexer must be configured")
	}
	for name, indexer := range c.Indexers {
		if indexer.URL == "" {
			errs = append(errs, fmt.Sprintf("indexers.%s.url: required", name))
		}
		if indexer.APIKey == "" {
			errs = append(errs, fmt.Sprintf("indexers.%s.api_key: required", name))
		}
	}

	// SABnzbd validation
	if c.Downloaders.SABnzbd != nil {
		if c.Downloaders.SABnzbd.URL == "" {
			errs = append(errs, "downloaders.sabnzbd.url: required when sabnzbd is configured")
		}
		if c.Downloaders.SABnzbd.APIKey == "" {
			errs = append(errs, "downloaders.sabnzbd.api_key: required when sabnzbd is configured")
		}
	}

	// AI validation
	if c.AI.Enabled {
		if !validAIProviders[c.AI.Provider] {
			errs = append(errs, fmt.Sprintf("ai.provider: must be one of ollama, anthropic; got %q", c.AI.Provider))
		}
	}

	// Library path warnings (non-fatal)
	if c.Libraries.Movies.Root != "" {
		if _, err := os.Stat(c.Libraries.Movies.Root); os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("libraries.movies.root: warning: directory %q does not exist", c.Libraries.Movies.Root))
		}
	}
	if c.Libraries.Series.Root != "" {
		if _, err := os.Stat(c.Libraries.Series.Root); os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("libraries.series.root: warning: directory %q does not exist", c.Libraries.Series.Root))
		}
	}

	return errs
}
