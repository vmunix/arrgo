package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	switch os.Args[1] {
	case "version", "-v", "--version":
		fmt.Printf("arrgo %s\n", version)
	case "serve":
		configPath := "config.toml"
		if len(os.Args) > 2 && os.Args[2] == "--config" && len(os.Args) > 3 {
			configPath = os.Args[3]
		}
		if err := runServe(configPath); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	case "init":
		fmt.Println("arrgo init: not yet implemented")
	case "status":
		runStatus(os.Args[2:])
	case "search":
		fmt.Println("arrgo search: not yet implemented")
	case "queue":
		fmt.Println("arrgo queue: not yet implemented")
	case "chat":
		fmt.Println("arrgo chat: not yet implemented")
	case "ask":
		fmt.Println("arrgo ask: not yet implemented")
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`arrgo - unified media automation

Usage:
  arrgo <command> [options]

Server:
  serve              Start API server and background jobs

Setup:
  init               Interactive setup wizard
  config check       Validate configuration
  migrate            Run database migrations

Commands:
  status             System status (health, disk, queue summary)
  search <query>     Search indexers for content
  queue              Show active downloads
  add <type> <id>    Add content by TMDB/TVDB ID
  grab <release-id>  Grab a specific release

AI Assistant:
  chat               Interactive conversation mode
  ask <question>     One-shot question

Other:
  version            Print version
  help               Show this help

Examples:
  arrgo init                    # Set up arrgo
  arrgo serve                   # Start the server
  arrgo search "The Matrix"     # Search for a movie
  arrgo chat                    # Start AI chat
  arrgo ask "why is my download stuck?"`)
}
