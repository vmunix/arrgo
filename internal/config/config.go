// Package config handles TOML configuration loading with environment variable substitution.
package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the root configuration structure.
type Config struct {
	Server      ServerConfig              `toml:"server"`
	Database    DatabaseConfig            `toml:"database"`
	Libraries   LibrariesConfig           `toml:"libraries"`
	Quality     QualityConfig             `toml:"quality"`
	Indexers    IndexersConfig            `toml:"indexers"`
	Downloaders DownloadersConfig         `toml:"downloaders"`
	Notifications NotificationsConfig     `toml:"notifications"`
	Overseerr   OverseerrConfig           `toml:"overseerr"`
	Compat      CompatConfig              `toml:"compat"`
	AI          AIConfig                  `toml:"ai"`
}

type ServerConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	LogLevel string `toml:"log_level"`
}

type DatabaseConfig struct {
	Path string `toml:"path"`
}

type LibrariesConfig struct {
	Movies LibraryConfig `toml:"movies"`
	Series LibraryConfig `toml:"series"`
}

type LibraryConfig struct {
	Root   string `toml:"root"`
	Naming string `toml:"naming"`
}

type QualityConfig struct {
	Default  string                      `toml:"default"`
	Profiles map[string]QualityProfile   `toml:"profiles"`
}

type QualityProfile struct {
	Accept []string `toml:"accept"`
}

type IndexersConfig struct {
	Prowlarr *ProwlarrConfig `toml:"prowlarr"`
}

type ProwlarrConfig struct {
	URL    string `toml:"url"`
	APIKey string `toml:"api_key"`
}

type DownloadersConfig struct {
	SABnzbd     *SABnzbdConfig     `toml:"sabnzbd"`
	QBittorrent *QBittorrentConfig `toml:"qbittorrent"`
}

type SABnzbdConfig struct {
	URL      string `toml:"url"`
	APIKey   string `toml:"api_key"`
	Category string `toml:"category"`
}

type QBittorrentConfig struct {
	URL      string `toml:"url"`
	Username string `toml:"username"`
	Password string `toml:"password"`
}

type NotificationsConfig struct {
	Plex *PlexConfig `toml:"plex"`
}

type PlexConfig struct {
	URL       string   `toml:"url"`
	Token     string   `toml:"token"`
	Libraries []string `toml:"libraries"`
}

type OverseerrConfig struct {
	Enabled      bool          `toml:"enabled"`
	URL          string        `toml:"url"`
	APIKey       string        `toml:"api_key"`
	SyncInterval time.Duration `toml:"sync_interval"`
}

type CompatConfig struct {
	APIKey string `toml:"api_key"`
	Radarr bool   `toml:"radarr"`
	Sonarr bool   `toml:"sonarr"`
}

type AIConfig struct {
	Enabled   bool             `toml:"enabled"`
	Provider  string           `toml:"provider"`
	Ollama    *OllamaConfig    `toml:"ollama"`
	Anthropic *AnthropicConfig `toml:"anthropic"`
}

type OllamaConfig struct {
	URL   string `toml:"url"`
	Model string `toml:"model"`
}

type AnthropicConfig struct {
	APIKey string `toml:"api_key"`
	Model  string `toml:"model"`
}

// Load reads and parses the configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	// Substitute environment variables
	content := substituteEnvVars(string(data))

	var cfg Config
	if _, err := toml.Decode(content, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Apply defaults
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8484
	}
	if cfg.Server.LogLevel == "" {
		cfg.Server.LogLevel = "info"
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "./data/arrgo.db"
	}

	return &cfg, nil
}

// substituteEnvVars replaces ${VAR_NAME} with environment variable values.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func substituteEnvVars(content string) string {
	return envVarPattern.ReplaceAllStringFunc(content, func(match string) string {
		varName := match[2 : len(match)-1] // Strip ${ and }
		if value, ok := os.LookupEnv(varName); ok {
			return value
		}
		return match // Leave unchanged if not found
	})
}
