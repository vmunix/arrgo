package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var plexCmd = &cobra.Command{
	Use:   "plex",
	Short: "Plex media server commands",
}

var plexStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Plex connection status and libraries",
	RunE:  runPlexStatusCmd,
}

var plexScanCmd = &cobra.Command{
	Use:   "scan [libraries...]",
	Short: "Trigger Plex library scan",
	Long:  "Trigger a scan of Plex libraries. Specify library names to scan specific libraries, or use --all to scan all libraries.",
	RunE:  runPlexScanCmd,
}

var plexScanAll bool

var plexListCmd = &cobra.Command{
	Use:   "list [library]",
	Short: "List Plex library contents",
	Long:  "List items in a Plex library with arrgo tracking status. If no library specified, lists all libraries.",
	RunE:  runPlexListCmd,
}

var plexListVerbose bool

var plexSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search Plex libraries",
	Long:  "Search for items across all Plex libraries.",
	Args:  cobra.ExactArgs(1),
	RunE:  runPlexSearchCmd,
}

func init() {
	rootCmd.AddCommand(plexCmd)
	plexCmd.AddCommand(plexStatusCmd)
	plexCmd.AddCommand(plexScanCmd)
	plexCmd.AddCommand(plexListCmd)
	plexCmd.AddCommand(plexSearchCmd)

	plexScanCmd.Flags().BoolVar(&plexScanAll, "all", false, "Scan all libraries")
	plexListCmd.Flags().BoolVarP(&plexListVerbose, "verbose", "v", false, "Show detailed output")
}

func runPlexStatusCmd(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)
	status, err := client.PlexStatus()
	if err != nil {
		return fmt.Errorf("plex status failed: %w", err)
	}

	if jsonOutput {
		printJSON(status)
		return nil
	}

	printPlexStatusHuman(status)
	return nil
}

func runPlexScanCmd(cmd *cobra.Command, args []string) error {
	// Require either --all or library names
	if !plexScanAll && len(args) == 0 {
		return fmt.Errorf("specify library names or use --all")
	}

	client := NewClient(serverURL)

	// If --all, pass empty slice to scan all
	libraries := args
	if plexScanAll {
		libraries = nil
	}

	resp, err := client.PlexScan(libraries)
	if err != nil {
		return fmt.Errorf("plex scan failed: %w", err)
	}

	if jsonOutput {
		printJSON(resp)
		return nil
	}

	if len(resp.Scanned) == 0 {
		fmt.Println("No libraries scanned")
		return nil
	}

	fmt.Println("Scan triggered for:")
	for _, lib := range resp.Scanned {
		fmt.Printf("  %s\n", lib)
	}
	return nil
}

func runPlexListCmd(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	// If no library specified, show all libraries with counts
	if len(args) == 0 {
		status, err := client.PlexStatus()
		if err != nil {
			return fmt.Errorf("plex status failed: %w", err)
		}

		if jsonOutput {
			printJSON(status.Libraries)
			return nil
		}

		fmt.Println("Libraries:")
		for _, lib := range status.Libraries {
			fmt.Printf("  %-12s %4d items\n", lib.Title, lib.ItemCount)
		}
		fmt.Println("\nUse 'arrgo plex list <library>' to see contents")
		return nil
	}

	// List specific library
	library := args[0]
	resp, err := client.PlexListLibrary(library)
	if err != nil {
		return fmt.Errorf("plex list failed: %w", err)
	}

	if jsonOutput {
		printJSON(resp)
		return nil
	}

	fmt.Printf("%s (%d items):\n", resp.Library, resp.Total)
	for _, item := range resp.Items {
		status := "✗ not tracked"
		if item.Tracked {
			status = "✓ tracked"
		}

		if plexListVerbose {
			fmt.Printf("\n  %s (%d)\n", item.Title, item.Year)
			fmt.Printf("    Path: %s\n", item.FilePath)
			fmt.Printf("    Added: %s | %s\n", formatTimeAgo(item.AddedAt), status)
			if item.ContentID != nil {
				fmt.Printf("    Content ID: %d\n", *item.ContentID)
			}
		} else {
			fmt.Printf("  %-45s %s\n", fmt.Sprintf("%s (%d)", item.Title, item.Year), status)
		}
	}

	return nil
}

func runPlexSearchCmd(cmd *cobra.Command, args []string) error {
	query := args[0]
	client := NewClient(serverURL)

	resp, err := client.PlexSearch(query)
	if err != nil {
		return fmt.Errorf("plex search failed: %w", err)
	}

	if jsonOutput {
		printJSON(resp)
		return nil
	}

	if resp.Total == 0 {
		fmt.Printf("No results for %q\n", query)
		return nil
	}

	fmt.Printf("Results for %q (%d items):\n", query, resp.Total)
	for _, item := range resp.Items {
		status := "✗ not tracked"
		if item.Tracked {
			status = "✓ tracked"
		}

		fmt.Printf("\n  %s (%d)\n", item.Title, item.Year)
		if item.FilePath != "" {
			fmt.Printf("    Path: %s\n", item.FilePath)
		}
		fmt.Printf("    Added: %s | %s\n", formatTimeAgo(item.AddedAt), status)
		if item.ContentID != nil {
			fmt.Printf("    Content ID: %d\n", *item.ContentID)
		}
	}

	return nil
}

func printPlexStatusHuman(s *PlexStatusResponse) {
	if s.Error != "" && !s.Connected {
		if s.Error == "Plex not configured" {
			fmt.Println("Plex: not configured")
			fmt.Println()
			fmt.Println("Configure in config.toml:")
			fmt.Println("  [notifications.plex]")
			fmt.Println("  url = \"http://localhost:32400\"")
			fmt.Println("  token = \"your-token\"")
		} else {
			fmt.Printf("Plex: connection failed\n")
			fmt.Printf("  Error: %s\n", s.Error)
		}
		return
	}

	fmt.Printf("Plex: %s (%s)\n", s.ServerName, s.Version)
	fmt.Println()

	if len(s.Libraries) == 0 {
		fmt.Println("No libraries found")
		return
	}

	fmt.Println("Libraries:")
	for _, lib := range s.Libraries {
		status := ""
		if lib.Refreshing {
			status = " (scanning)"
		}

		scannedAgo := formatTimeAgo(lib.ScannedAt)

		fmt.Printf("  %-12s %4d items   %-24s scanned %s%s\n",
			lib.Title, lib.ItemCount, lib.Location, scannedAgo, status)
	}
}

func formatTimeAgo(unixTime int64) string {
	if unixTime == 0 {
		return "never"
	}

	t := time.Unix(unixTime, 0)
	ago := time.Since(t)

	switch {
	case ago < time.Minute:
		return "just now"
	case ago < time.Hour:
		mins := int(ago.Minutes())
		if mins == 1 {
			return "1m ago"
		}
		return fmt.Sprintf("%dm ago", mins)
	case ago < 24*time.Hour:
		hours := int(ago.Hours())
		if hours == 1 {
			return "1h ago"
		}
		return fmt.Sprintf("%dh ago", hours)
	default:
		days := int(ago.Hours() / 24)
		if days == 1 {
			return "1d ago"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}
