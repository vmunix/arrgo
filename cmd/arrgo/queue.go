package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Show active downloads",
	RunE:  runQueueCmd,
}

var queueCancelCmd = &cobra.Command{
	Use:   "cancel <id>",
	Short: "Cancel a download",
	Long:  "Cancels the download and removes it from the download client. Use --delete to also remove downloaded files.",
	Args:  cobra.ExactArgs(1),
	RunE:  runQueueCancel,
}

var queueShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show detailed download info",
	Args:  cobra.ExactArgs(1),
	RunE:  runQueueShow,
}

var queueRetryCmd = &cobra.Command{
	Use:   "retry <id>",
	Short: "Retry a failed download",
	Long:  "Re-searches indexers for the content and grabs the best matching release.",
	Args:  cobra.ExactArgs(1),
	RunE:  runQueueRetry,
}

func init() {
	rootCmd.AddCommand(queueCmd)
	queueCmd.Flags().BoolP("all", "a", false, "Include terminal states (cleaned, failed)")
	queueCmd.Flags().StringP("state", "s", "", "Filter by state (queued, downloading, completed, importing, imported, cleaned, failed)")

	queueCancelCmd.Flags().BoolP("delete", "d", false, "Also delete downloaded files")
	queueCmd.AddCommand(queueCancelCmd)
	queueCmd.AddCommand(queueShowCmd)
	queueCmd.AddCommand(queueRetryCmd)
}

func runQueueCancel(cmd *cobra.Command, args []string) error {
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

	if deleteFiles {
		fmt.Printf("Download %d canceled (files deleted)\n", id)
	} else {
		fmt.Printf("Download %d canceled\n", id)
	}
	return nil
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

func runQueueShow(cmd *cobra.Command, args []string) error {
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

func runQueueRetry(cmd *cobra.Command, args []string) error {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid ID: %s", args[0])
	}

	client := NewClient(serverURL)

	fmt.Printf("Retrying download #%d...\n", id)
	result, err := client.RetryDownload(id)
	if err != nil {
		return fmt.Errorf("retry failed: %w", err)
	}

	if jsonOutput {
		printJSON(result)
		return nil
	}

	fmt.Printf("Retry queued: %s\n", result.ReleaseName)
	fmt.Println("Use 'arrgo queue' to monitor progress")
	return nil
}
