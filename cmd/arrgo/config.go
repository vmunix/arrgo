package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmunix/arrgo/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration management",
}

var configTestCmd = &cobra.Command{
	Use:   "test [path]",
	Short: "Validate configuration file",
	Long:  "Validates config.toml syntax, required fields, and environment variable substitution without starting the server.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runConfigTest,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configTestCmd)
}

func runConfigTest(cmd *cobra.Command, args []string) error {
	path := "config.toml"
	if len(args) > 0 {
		path = args[0]
	}

	fmt.Printf("Validating %s...\n\n", path)

	cfg, err := config.Load(path)
	if err != nil {
		var configErr *config.Error
		if errors.As(err, &configErr) {
			printConfigErrors(configErr)
			return fmt.Errorf("configuration invalid")
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	printConfigSummary(cfg)
	fmt.Println("\nConfiguration valid!")
	return nil
}

func printConfigErrors(e *config.Error) {
	if len(e.Missing) > 0 {
		fmt.Println("Missing environment variables:")
		for _, m := range e.Missing {
			fmt.Printf("  - %s\n", m)
		}
		fmt.Println()
	}

	if len(e.Errors) > 0 {
		fmt.Println("Validation errors:")
		for _, err := range e.Errors {
			fmt.Printf("  - %s\n", err)
		}
		fmt.Println()
	}
}

func printConfigSummary(cfg *config.Config) {
	fmt.Println("Configuration Summary:")
	fmt.Printf("  Server:     %s:%d (log: %s)\n", cfg.Server.Host, cfg.Server.Port, cfg.Server.LogLevel)
	fmt.Printf("  Database:   %s\n", cfg.Database.Path)

	// Libraries
	libs := []string{}
	if cfg.Libraries.Movies.Root != "" {
		libs = append(libs, "movies")
	}
	if cfg.Libraries.Series.Root != "" {
		libs = append(libs, "series")
	}
	fmt.Printf("  Libraries:  %s\n", strings.Join(libs, ", "))

	// Indexers
	indexerNames := make([]string, 0, len(cfg.Indexers))
	for name := range cfg.Indexers {
		indexerNames = append(indexerNames, name)
	}
	fmt.Printf("  Indexers:   %s\n", strings.Join(indexerNames, ", "))

	// Quality profiles
	profileNames := make([]string, 0, len(cfg.Quality.Profiles))
	for name := range cfg.Quality.Profiles {
		profileNames = append(profileNames, name)
	}
	if len(profileNames) > 0 {
		fmt.Printf("  Profiles:   %s", strings.Join(profileNames, ", "))
		if cfg.Quality.Default != "" {
			fmt.Printf(" (default: %s)", cfg.Quality.Default)
		}
		fmt.Println()
	}

	// Downloaders
	downloaders := []string{}
	if cfg.Downloaders.SABnzbd != nil {
		downloaders = append(downloaders, "sabnzbd")
	}
	if cfg.Downloaders.QBittorrent != nil {
		downloaders = append(downloaders, "qbittorrent")
	}
	if len(downloaders) > 0 {
		fmt.Printf("  Downloaders: %s\n", strings.Join(downloaders, ", "))
	}

	// Integrations
	integrations := []string{}
	if cfg.Notifications.Plex != nil {
		integrations = append(integrations, "plex")
	}
	if cfg.TMDB != nil && cfg.TMDB.APIKey != "" {
		integrations = append(integrations, "tmdb")
	}
	if cfg.Compat.Radarr || cfg.Compat.Sonarr {
		integrations = append(integrations, "compat-api")
	}
	if len(integrations) > 0 {
		fmt.Printf("  Integrations: %s\n", strings.Join(integrations, ", "))
	}
}
