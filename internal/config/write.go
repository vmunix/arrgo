// internal/config/write.go
package config

import (
	_ "embed"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

//go:embed default_config.toml
var defaultConfig string

// WriteDefault writes the example config to the specified path.
// Creates parent directories if needed.
func WriteDefault(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultConfig), 0644)
}

// Write serializes the config to TOML and writes it to the specified path.
func (c *Config) Write(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(c); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
