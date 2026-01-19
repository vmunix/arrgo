package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

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

	// TODO: Prompt for values and write config
	fmt.Println("Not yet implemented")
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
