package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

// Common flags
type commonFlags struct {
	server string
	json   bool
}

func parseCommonFlags(fs *flag.FlagSet, args []string) commonFlags {
	var f commonFlags
	fs.StringVar(&f.server, "server", "http://localhost:8484", "Server URL")
	fs.BoolVar(&f.json, "json", false, "Output as JSON")
	fs.Parse(args)
	return f
}

func runStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	flags := parseCommonFlags(fs, args)

	client := NewClient(flags.server)
	status, err := client.Status()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if flags.json {
		printJSON(status)
		return
	}

	printStatusHuman(flags.server, status)
}

func printStatusHuman(server string, s *StatusResponse) {
	fmt.Printf("Server:     %s (%s)\n", server, s.Status)
	fmt.Printf("Version:    %s\n", s.Version)
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func runQueue(args []string) {
	fs := flag.NewFlagSet("queue", flag.ExitOnError)
	var showAll bool
	fs.BoolVar(&showAll, "all", false, "Include completed/imported downloads")
	flags := parseCommonFlags(fs, args)

	client := NewClient(flags.server)
	downloads, err := client.Downloads(!showAll)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if flags.json {
		printJSON(downloads)
		return
	}

	printQueueHuman(downloads)
}

func printQueueHuman(d *ListDownloadsResponse) {
	if len(d.Items) == 0 {
		fmt.Println("No downloads in queue")
		return
	}

	fmt.Printf("Downloads (%d):\n\n", d.Total)
	fmt.Printf("  # │ %-40s │ %-12s │ %s\n", "TITLE", "STATUS", "INDEXER")
	fmt.Println("────┼──────────────────────────────────────────┼──────────────┼─────────")

	for i, dl := range d.Items {
		title := dl.ReleaseName
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		fmt.Printf(" %2d │ %-40s │ %-12s │ %s\n", i+1, title, dl.Status, dl.Indexer)
	}
}
