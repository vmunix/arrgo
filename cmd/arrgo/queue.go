package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Show active downloads",
	RunE:  runQueueCmd,
}

func init() {
	rootCmd.AddCommand(queueCmd)
	queueCmd.Flags().BoolP("all", "a", false, "Include terminal states (cleaned, failed)")
	queueCmd.Flags().StringP("state", "s", "", "Filter by state (queued, downloading, completed, imported, cleaned, failed)")
}

func runQueueCmd(cmd *cobra.Command, args []string) error {
	showAll, _ := cmd.Flags().GetBool("all")
	stateFilter, _ := cmd.Flags().GetString("state")

	client := NewClient(serverURL)
	downloads, err := client.Downloads(!showAll)
	if err != nil {
		return fmt.Errorf("queue fetch failed: %w", err)
	}

	// Filter by state if specified
	if stateFilter != "" {
		filtered := make([]DownloadResponse, 0)
		for _, dl := range downloads.Items {
			if strings.EqualFold(dl.Status, stateFilter) {
				filtered = append(filtered, dl)
			}
		}
		downloads.Items = filtered
		downloads.Total = len(filtered)
	}

	if jsonOutput {
		printJSON(downloads)
		return nil
	}

	if showAll {
		printQueueAll(downloads)
	} else {
		printQueueActive(downloads)
	}
	return nil
}

func printQueueActive(d *ListDownloadsResponse) {
	if len(d.Items) == 0 {
		fmt.Println("No active downloads")
		return
	}

	fmt.Printf("Active Downloads (%d):\n\n", d.Total)
	fmt.Printf("  %-4s %-12s %-44s %s\n", "ID", "STATE", "RELEASE", "PROGRESS")
	fmt.Println("  " + strings.Repeat("-", 70))

	for _, dl := range d.Items {
		title := dl.ReleaseName
		if len(title) > 44 {
			title = title[:41] + "..."
		}
		progress := "-"
		if dl.Status == "downloading" {
			progress = "..." // Would need live data from SABnzbd
		} else if dl.Status == "completed" {
			progress = "100%"
		}
		fmt.Printf("  %-4d %-12s %-44s %s\n", dl.ID, dl.Status, title, progress)
	}
}

func printQueueAll(d *ListDownloadsResponse) {
	if len(d.Items) == 0 {
		fmt.Println("No downloads")
		return
	}

	fmt.Printf("All Downloads (%d):\n\n", d.Total)
	fmt.Printf("  %-4s %-12s %-40s %-12s\n", "ID", "STATE", "RELEASE", "COMPLETED")
	fmt.Println("  " + strings.Repeat("-", 72))

	for _, dl := range d.Items {
		title := dl.ReleaseName
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		completed := "-"
		if dl.CompletedAt != nil {
			if t, err := time.Parse(time.RFC3339, *dl.CompletedAt); err == nil {
				completed = formatTimeAgo(t.Unix())
			}
		}
		fmt.Printf("  %-4d %-12s %-40s %-12s\n", dl.ID, dl.Status, title, completed)
	}
}
