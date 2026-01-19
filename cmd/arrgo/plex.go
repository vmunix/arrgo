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

func init() {
	rootCmd.AddCommand(plexCmd)
	plexCmd.AddCommand(plexStatusCmd)
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
