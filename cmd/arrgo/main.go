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
	case "init":
		runInit(os.Args[2:])
	case "status":
		runStatus(os.Args[2:])
	case "search":
		runSearch(os.Args[2:])
	case "parse":
		runParse(os.Args[2:])
	case "queue":
		runQueue(os.Args[2:])
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
	fmt.Println(`arrgo - CLI client for arrgo media automation

Usage:
  arrgo <command> [options]

Setup:
  init               Interactive setup wizard

Commands:
  status             System status (health, disk, queue summary)
  search [flags] <query>  Search indexers for content
  parse <release>    Parse release name (local, no server needed)
  queue              Show active downloads

AI Assistant:
  chat               Interactive conversation mode
  ask <question>     One-shot question

Other:
  version            Print version
  help               Show this help

Server:
  Run 'arrgod' to start the server daemon.

Examples:
  arrgo init                    # Set up arrgo
  arrgo search "The Matrix"     # Search for a movie
  arrgo parse "Movie.2024..."   # Parse a release name
  arrgo status                  # Check server status`)
}
