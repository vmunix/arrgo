package main

import (
	"flag"
	"fmt"
	"os"
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
