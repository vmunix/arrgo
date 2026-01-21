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
	queueCmd.Flags().StringP("state", "s", "", "Filter by state (queued, downloading, completed, importing, imported, cleaned, failed)")
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
		for i := range downloads.Items {
			if strings.EqualFold(downloads.Items[i].Status, stateFilter) {
				filtered = append(filtered, downloads.Items[i])
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
	fmt.Printf("  %-4s %-12s %-50s %-8s %s\n", "ID", "STATE", "RELEASE", "PROGRESS", "ETA")
	fmt.Println("  " + strings.Repeat("-", 90))

	for i := range d.Items {
		dl := &d.Items[i]
		title := dl.ReleaseName
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		progress := "-"
		if dl.Progress != nil {
			progress = fmt.Sprintf("%.1f%%", *dl.Progress)
		} else if dl.Status == "completed" {
			progress = "100%"
		}
		eta := "-"
		if dl.ETA != nil {
			eta = *dl.ETA
		}
		fmt.Printf("  %-4d %-12s %-50s %-8s %s\n", dl.ID, dl.Status, title, progress, eta)
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

	for i := range d.Items {
		dl := &d.Items[i]
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
