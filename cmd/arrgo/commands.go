package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/arrgo/arrgo/pkg/release"
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
	_ = fs.Parse(args)
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
	_ = enc.Encode(v)
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

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func prompt(message string) string {
	fmt.Print(message)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func runSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	var contentType, profile, grabFlag string
	var verbose bool
	fs.StringVar(&contentType, "type", "", "Content type (movie or series)")
	fs.StringVar(&profile, "profile", "", "Quality profile")
	fs.StringVar(&grabFlag, "grab", "", "Grab release: number or 'best'")
	fs.BoolVar(&verbose, "verbose", false, "Show extended info (indexer, group)")
	flags := parseCommonFlags(fs, args)

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: arrgo search [--verbose] [--type movie|series] [--grab N|best] <query>")
		os.Exit(1)
	}
	query := strings.Join(remaining, " ")

	client := NewClient(flags.server)
	results, err := client.Search(query, contentType, profile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if flags.json {
		printJSON(results)
		return
	}

	if len(results.Releases) == 0 {
		fmt.Println("No releases found")
		return
	}

	printSearchHuman(query, results, verbose)

	// Handle grab
	var grabIndex int
	if grabFlag == "best" {
		grabIndex = 1
	} else if grabFlag != "" {
		grabIndex, _ = strconv.Atoi(grabFlag)
	} else if !flags.json {
		// Interactive prompt
		input := prompt(fmt.Sprintf("\nGrab? [1-%d, n]: ", len(results.Releases)))
		if input == "" || input == "n" || input == "N" {
			return
		}
		grabIndex, _ = strconv.Atoi(input)
	}

	if grabIndex < 1 || grabIndex > len(results.Releases) {
		return
	}

	// Grab the selected release
	selected := results.Releases[grabIndex-1]
	grabRelease(client, selected, contentType, profile)
}

func printSearchHuman(query string, r *SearchResponse, verbose bool) {
	fmt.Printf("Found %d releases for %q:\n\n", len(r.Releases), query)
	fmt.Printf("  # │ %-42s │ %8s │ %5s\n", "RELEASE", "SIZE", "SCORE")
	fmt.Println("────┼────────────────────────────────────────────┼──────────┼───────")

	for i, rel := range r.Releases {
		title := rel.Title
		if len(title) > 42 {
			title = title[:39] + "..."
		}
		fmt.Printf(" %2d │ %-42s │ %8s │ %5d\n",
			i+1, title, formatSize(rel.Size), rel.Score)

		// Parse release to get quality info for badges
		info := release.Parse(rel.Title)
		badges := buildBadges(info)
		if badges != "" {
			fmt.Printf("    │ %s\n", badges)
		}

		// Verbose mode: show indexer, group, and service
		if verbose {
			var parts []string
			if rel.Indexer != "" {
				parts = append(parts, "Indexer: "+rel.Indexer)
			}
			if info.Group != "" {
				parts = append(parts, "Group: "+info.Group)
			}
			if info.Service != "" {
				parts = append(parts, "Service: "+info.Service)
			}
			if len(parts) > 0 {
				fmt.Printf("    │ %s\n", strings.Join(parts, "  "))
			}
		}
	}

	if len(r.Errors) > 0 {
		fmt.Printf("\nWarnings: %s\n", strings.Join(r.Errors, ", "))
	}
}

// buildBadges creates a formatted string of quality badges from parsed release info.
// Badge order: resolution, source, codec, HDR, audio, remux, edition
// Only includes badges for detected (non-unknown/non-empty) attributes.
func buildBadges(info *release.Info) string {
	var badges []string

	// Resolution
	if info.Resolution != release.ResolutionUnknown {
		badges = append(badges, "["+info.Resolution.String()+"]")
	}

	// Source - format nicely
	if info.Source != release.SourceUnknown {
		source := info.Source.String()
		// Capitalize nicely
		switch info.Source {
		case release.SourceBluRay:
			source = "BluRay"
		case release.SourceWEBDL:
			source = "WEB-DL"
		case release.SourceWEBRip:
			source = "WEBRip"
		case release.SourceHDTV:
			source = "HDTV"
		}
		badges = append(badges, "["+source+"]")
	}

	// Codec
	if info.Codec != release.CodecUnknown {
		badges = append(badges, "["+info.Codec.String()+"]")
	}

	// HDR
	if info.HDR != release.HDRNone {
		badges = append(badges, "["+info.HDR.String()+"]")
	}

	// Audio
	if info.Audio != release.AudioUnknown {
		badges = append(badges, "["+info.Audio.String()+"]")
	}

	// Remux
	if info.IsRemux {
		badges = append(badges, "[Remux]")
	}

	// Edition
	if info.Edition != "" {
		badges = append(badges, "["+info.Edition+"]")
	}

	return strings.Join(badges, " ")
}

func grabRelease(client *Client, rel ReleaseResponse, contentType, profile string) {
	// Parse release name to get title/year
	info := release.Parse(rel.Title)
	if info.Title == "" {
		fmt.Fprintln(os.Stderr, "Error: Could not parse release title")
		return
	}

	// Determine content type if not specified
	if contentType == "" {
		if info.Season > 0 || info.Episode > 0 {
			contentType = "series"
		} else {
			contentType = "movie"
		}
	}

	// Show what we parsed
	fmt.Printf("\nParsed: %s (%d)\n", info.Title, info.Year)

	input := prompt("Confirm? [Y/n]: ")
	if input != "" && input != "y" && input != "Y" {
		fmt.Println("Cancelled")
		return
	}

	// Default profile
	if profile == "" {
		profile = "hd"
	}

	// Try to find existing content first
	content, err := client.FindContent(contentType, info.Title, info.Year)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding content: %v\n", err)
		return
	}

	if content != nil {
		fmt.Printf("Found in library (ID: %d)\n", content.ID)
	} else {
		// Create new content entry
		content, err = client.AddContent(contentType, info.Title, info.Year, profile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error adding content: %v\n", err)
			return
		}
		fmt.Printf("Added to library (ID: %d)\n", content.ID)
	}

	// Grab the release
	grab, err := client.Grab(content.ID, rel.DownloadURL, rel.Title, rel.Indexer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error grabbing: %v\n", err)
		return
	}
	fmt.Printf("Download started (ID: %d)\n", grab.DownloadID)
}
