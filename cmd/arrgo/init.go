package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

type initConfig struct {
	ProwlarrURL     string
	ProwlarrAPIKey  string
	SABnzbdURL      string
	SABnzbdAPIKey   string
	SABnzbdCategory string
	MoviesPath      string
	SeriesPath      string
}

func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	force := fs.Bool("force", false, "Overwrite existing config.toml")
	_ = fs.Parse(args)

	configPath := "config.toml"

	// Check if config exists
	if _, err := os.Stat(configPath); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "config.toml already exists. Use --force to overwrite.\n")
		os.Exit(1)
	}

	fmt.Println("arrgo setup wizard")
	fmt.Println()

	cfg := gatherConfig()

	// TODO: Write config file
	_ = cfg
}

func gatherConfig() initConfig {
	var cfg initConfig

	cfg.ProwlarrURL = promptWithDefault("Prowlarr URL", "http://localhost:9696")
	cfg.ProwlarrAPIKey = promptRequired("Prowlarr API Key")
	fmt.Println()

	cfg.SABnzbdURL = promptWithDefault("SABnzbd URL", "http://localhost:8085")
	cfg.SABnzbdAPIKey = promptRequired("SABnzbd API Key")
	cfg.SABnzbdCategory = promptWithDefault("SABnzbd Category", "arrgo")
	fmt.Println()

	cfg.MoviesPath = promptWithDefault("Movies path", "/movies")
	cfg.SeriesPath = promptWithDefault("Series path", "/tv")

	return cfg
}

// promptWithDefault shows a prompt with default value in brackets.
// Returns the user's input, or the default if input is empty.
func promptWithDefault(label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	return input
}

// promptRequired prompts until a non-empty value is provided.
func promptRequired(label string) string {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s: ", label)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			return input
		}
		fmt.Println("  Value required")
	}
}
