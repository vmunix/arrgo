package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Valid download states for --state flag validation
var validStates = []string{"queued", "downloading", "completed", "importing", "imported", "cleaned", "failed"}

var downloadsCmd = &cobra.Command{
	Use:   "downloads",
	Short: "Show and manage downloads",
	Long: `Show and manage downloads.

Examples:
  arrgo downloads                     # Show active downloads
  arrgo downloads --all               # Include terminal states (cleaned, failed)
  arrgo downloads --state failed      # Filter by state
  arrgo downloads show 42             # Show detailed info for download #42
  arrgo downloads cancel 42           # Cancel download #42
  arrgo downloads cancel 42 --delete  # Cancel and delete files
  arrgo downloads retry 42            # Retry a failed download`,
	RunE: runDownloadsCmd,
}

var downloadsCancelCmd = &cobra.Command{
	Use:   "cancel <id>",
	Short: "Cancel a download",
	Long:  "Cancels the download and removes it from the download client. Use --delete to also remove downloaded files.",
	Args:  cobra.ExactArgs(1),
	RunE:  runDownloadsCancel,
}

var downloadsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show detailed download info",
	Args:  cobra.ExactArgs(1),
	RunE:  runDownloadsShow,
}

var downloadsRetryCmd = &cobra.Command{
	Use:   "retry <id>",
	Short: "Retry a failed download",
	Long:  "Re-searches indexers for the content and grabs the best matching release.",
	Args:  cobra.ExactArgs(1),
	RunE:  runDownloadsRetry,
}

func init() {
	rootCmd.AddCommand(downloadsCmd)
	downloadsCmd.Flags().BoolP("all", "a", false, "Include terminal states (cleaned, failed)")
	downloadsCmd.Flags().StringP("state", "s", "", "Filter by state (queued, downloading, completed, importing, imported, cleaned, failed)")

	downloadsCancelCmd.Flags().BoolP("delete", "d", false, "Also delete downloaded files")
	downloadsCmd.AddCommand(downloadsCancelCmd)
	downloadsCmd.AddCommand(downloadsShowCmd)
	downloadsCmd.AddCommand(downloadsRetryCmd)
}

func runDownloadsCancel(cmd *cobra.Command, args []string) error {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid ID: %s", args[0])
	}

	deleteFiles := false
	if cmd != nil {
		deleteFiles, _ = cmd.Flags().GetBool("delete")
	}

	client := NewClient(serverURL)
	if err := client.CancelDownload(id, deleteFiles); err != nil {
		return fmt.Errorf("cancel failed: %w", err)
	}

	if !quietOutput {
		if deleteFiles {
			fmt.Printf("Download %d canceled (files deleted)\n", id)
		} else {
			fmt.Printf("Download %d canceled\n", id)
		}
	}
	return nil
}

func runDownloadsCmd(cmd *cobra.Command, args []string) error {
	showAll, _ := cmd.Flags().GetBool("all")
	stateFilter, _ := cmd.Flags().GetString("state")

	// Validate state filter
	if stateFilter != "" {
		valid := false
		for _, s := range validStates {
			if strings.EqualFold(stateFilter, s) {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("invalid state %q, valid states: %s", stateFilter, strings.Join(validStates, ", "))
		}
	}

	client := NewClient(serverURL)
	downloads, err := client.Downloads(!showAll)
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
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
		printDownloadsAll(downloads)
	} else {
		printDownloadsActive(downloads)
	}
	return nil
}

func printDownloadsActive(d *ListDownloadsResponse) {
	if len(d.Items) == 0 {
		fmt.Println("No active downloads")
		return
	}

	fmt.Printf("Active Downloads (%d):\n\n", d.Total)
	fmt.Printf("  %-4s %-12s %-46s %-8s %-10s %s\n", "ID", "STATE", "RELEASE", "PROGRESS", "SPEED", "ETA")
	fmt.Println("  " + strings.Repeat("-", 100))

	for i := range d.Items {
		dl := &d.Items[i]
		title := dl.ReleaseName
		if len(title) > 46 {
			title = title[:43] + "..."
		}
		progress := "-"
		if dl.Progress != nil {
			progress = fmt.Sprintf("%.1f%%", *dl.Progress)
		} else if dl.Status == "completed" {
			progress = "100%"
		}
		speed := "-"
		if dl.Speed != nil && *dl.Speed > 0 {
			speed = formatSpeed(*dl.Speed)
		}
		eta := "-"
		if dl.ETA != nil {
			eta = *dl.ETA
		}
		fmt.Printf("  %-4d %-12s %-46s %-8s %-10s %s\n", dl.ID, dl.Status, title, progress, speed, eta)
	}
}

func formatSpeed(bytesPerSec int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case bytesPerSec >= GB:
		return fmt.Sprintf("%.1f GB/s", float64(bytesPerSec)/GB)
	case bytesPerSec >= MB:
		return fmt.Sprintf("%.1f MB/s", float64(bytesPerSec)/MB)
	case bytesPerSec >= KB:
		return fmt.Sprintf("%.1f KB/s", float64(bytesPerSec)/KB)
	default:
		return fmt.Sprintf("%d B/s", bytesPerSec)
	}
}

func printDownloadsAll(d *ListDownloadsResponse) {
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

func runDownloadsShow(cmd *cobra.Command, args []string) error {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid ID: %s", args[0])
	}

	client := NewClient(serverURL)
	dl, err := client.Download(id)
	if err != nil {
		return fmt.Errorf("failed to fetch download: %w", err)
	}

	if jsonOutput {
		printJSON(dl)
		return nil
	}

	fmt.Printf("Download #%d\n\n", dl.ID)
	fmt.Printf("  %-12s %s\n", "Release:", dl.ReleaseName)
	fmt.Printf("  %-12s %d\n", "Content ID:", dl.ContentID)
	if dl.Season != nil {
		if dl.IsCompleteSeason {
			fmt.Printf("  %-12s Season %d (complete season pack)\n", "Season:", *dl.Season)
		} else {
			fmt.Printf("  %-12s %d\n", "Season:", *dl.Season)
		}
	}
	fmt.Printf("  %-12s %s\n", "Status:", dl.Status)
	fmt.Printf("  %-12s %s\n", "Indexer:", dl.Indexer)
	fmt.Printf("  %-12s %s (%s)\n", "Client:", dl.Client, dl.ClientID)
	fmt.Printf("  %-12s %s\n", "Added:", dl.AddedAt)
	if dl.CompletedAt != nil {
		fmt.Printf("  %-12s %s\n", "Completed:", *dl.CompletedAt)
	}

	// Fetch and display events
	events, err := client.DownloadEvents(id)
	if err == nil && len(events.Items) > 0 {
		fmt.Printf("\n  Event History:\n")
		for _, e := range events.Items {
			t, _ := time.Parse(time.RFC3339, e.OccurredAt)
			fmt.Printf("    %s  %s\n", t.Format("15:04:05"), e.EventType)
		}
	}

	return nil
}

func runDownloadsRetry(cmd *cobra.Command, args []string) error {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid ID: %s", args[0])
	}

	client := NewClient(serverURL)

	if !quietOutput {
		fmt.Printf("Retrying download #%d...\n", id)
	}
	result, err := client.RetryDownload(id)
	if err != nil {
		return fmt.Errorf("retry failed: %w", err)
	}

	if jsonOutput {
		printJSON(result)
		return nil
	}

	fmt.Printf("Retry queued: %s\n", result.ReleaseName)
	fmt.Println("Use 'arrgo downloads' to monitor progress")
	return nil
}
