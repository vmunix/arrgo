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
	Server        ServerConfig        `toml:"server"`
	Database      DatabaseConfig      `toml:"database"`
	Libraries     LibrariesConfig     `toml:"libraries"`
	Quality       QualityConfig       `toml:"quality"`
	Indexers      IndexersConfig      `toml:"indexers"`
	Downloaders   DownloadersConfig   `toml:"downloaders"`
	Notifications NotificationsConfig `toml:"notifications"`
	Overseerr     OverseerrConfig     `toml:"overseerr"`
	Compat        CompatConfig        `toml:"compat"`
	AI            AIConfig            `toml:"ai"`
	Importer      ImporterConfig      `toml:"importer"`
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
	Default  string                    `toml:"default"`
	Profiles map[string]QualityProfile `toml:"profiles"`
}

type QualityProfile struct {
	Resolution  []string `toml:"resolution"`
	Sources     []string `toml:"sources"`
	Codecs      []string `toml:"codecs"`
	HDR         []string `toml:"hdr"`
	Audio       []string `toml:"audio"`
	PreferRemux bool     `toml:"prefer_remux"`
	Reject      []string `toml:"reject"`
}

// IndexersConfig is a map of indexer name to config.
// Parsed from [indexers.NAME] sections in TOML.
type IndexersConfig map[string]*NewznabConfig

type NewznabConfig struct {
	URL    string `toml:"url"`
	APIKey string `toml:"api_key"`
}

type DownloadersConfig struct {
	SABnzbd     *SABnzbdConfig     `toml:"sabnzbd"`
	QBittorrent *QBittorrentConfig `toml:"qbittorrent"`
}

type SABnzbdConfig struct {
	URL        string `toml:"url"`
	APIKey     string `toml:"api_key"`
	Category   string `toml:"category"`
	RemotePath string `toml:"remote_path"` // Path prefix as seen by SABnzbd (e.g., /data/usenet)
	LocalPath  string `toml:"local_path"`  // Corresponding path on this machine (e.g., /srv/data/usenet)
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
	URL        string   `toml:"url"`
	Token      string   `toml:"token"`
	Libraries  []string `toml:"libraries"`
	RemotePath string   `toml:"remote_path"` // Path prefix as seen by Plex (e.g., /data/media)
	LocalPath  string   `toml:"local_path"`  // Corresponding path on this machine (e.g., /srv/data/media)
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

type ImporterConfig struct {
	CleanupSource *bool `toml:"cleanup_source"`
}

// ShouldCleanupSource returns whether to delete source files after import.
// Defaults to true if not explicitly configured.
func (c *ImporterConfig) ShouldCleanupSource() bool {
	if c.CleanupSource == nil {
		return true // default
	}
	return *c.CleanupSource
}

// Load reads, parses, and validates the configuration file.
func Load(path string) (*Config, error) {
	cfg, missing, err := load(path)
	if err != nil {
		return nil, err
	}

	// Build ConfigError if any issues
	configErr := &ConfigError{Path: path, Missing: missing}

	// Run validation
	configErr.Errors = cfg.Validate()

	if configErr.HasErrors() {
		return nil, configErr
	}

	return cfg, nil
}

// LoadWithoutValidation reads and parses the config without validation.
// Useful for init commands or debugging.
func LoadWithoutValidation(path string) (*Config, error) {
	cfg, _, err := load(path)
	return cfg, err
}

// load is the internal loader that returns config, missing vars, and parse error.
func load(path string) (*Config, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading config: %w", err)
	}

	// Substitute environment variables
	content, missing := substituteEnvVars(string(data))

	var cfg Config
	if _, err := toml.Decode(content, &cfg); err != nil {
		return nil, nil, fmt.Errorf("parsing config: %w", err)
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

	return &cfg, missing, nil
}

// substituteEnvVars replaces ${VAR}, ${VAR:-default}, ${VAR:?error} patterns.
// Returns the substituted content and a list of missing/error variables.
var envVarPattern = regexp.MustCompile(`\$\{([^}:]+)(?:(:[-?])([^}]*))?\}`)

func substituteEnvVars(content string) (string, []string) {
	var missing []string

	result := envVarPattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := envVarPattern.FindStringSubmatch(match)
		varName := parts[1]
		modifier := parts[2]
		modValue := parts[3]

		value, exists := os.LookupEnv(varName)

		switch modifier {
		case ":-": // Default value
			if !exists || value == "" {
				return modValue
			}
			return value
		case ":?": // Required with error
			if !exists || value == "" {
				missing = append(missing, varName+": "+modValue)
				return match
			}
			return value
		default: // Simple substitution
			if exists {
				return value
			}
			missing = append(missing, varName)
			return match
		}
	})

	return result, missing
}
