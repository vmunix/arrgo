package main

import (
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var (
	serverURL  string
	jsonOutput bool
)

var rootCmd = &cobra.Command{
	Use:   "arrgo",
	Short: "CLI client for arrgo media automation",
	Long: `arrgo - CLI client for arrgo media automation

A unified CLI for searching indexers, managing downloads,
and automating your media library.

Run 'arrgod' to start the server daemon.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "http://localhost:8484", "Server URL")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	rootCmd.Version = version
	rootCmd.SetVersionTemplate("arrgo {{.Version}}\n")
}
