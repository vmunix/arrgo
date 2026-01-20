package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var importsCmd = &cobra.Command{
	Use:   "imports",
	Short: "Show pending imports and recent completions",
	RunE:  runImportsCmd,
}

func init() {
	rootCmd.AddCommand(importsCmd)
	importsCmd.Flags().Bool("pending", false, "Only show pending imports")
	importsCmd.Flags().Bool("recent", false, "Only show recent completions")
}

func runImportsCmd(cmd *cobra.Command, args []string) error {
	pendingOnly, _ := cmd.Flags().GetBool("pending")
	recentOnly, _ := cmd.Flags().GetBool("recent")

	client := NewClient(serverURL)

	// Get all downloads to filter
	downloads, err := client.Downloads(false)
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}

	// Split into pending (imported) and recent (cleaned)
	var pending, recent []DownloadResponse
	for _, dl := range downloads.Items {
		switch dl.Status {
		case "imported":
			pending = append(pending, dl)
		case "cleaned":
			// Only include if completed within last 24h
			if dl.CompletedAt != nil {
				if t, err := time.Parse(time.RFC3339, *dl.CompletedAt); err == nil {
					if time.Since(t) < 24*time.Hour {
						recent = append(recent, dl)
					}
				}
			}
		}
	}

	if jsonOutput {
		printJSON(map[string]any{
			"pending": pending,
			"recent":  recent,
		})
		return nil
	}

	if !recentOnly {
		printPendingImports(pending)
	}
	if !pendingOnly && !recentOnly {
		fmt.Println()
	}
	if !pendingOnly {
		printRecentImports(recent)
	}
	return nil
}

func printPendingImports(items []DownloadResponse) {
	fmt.Printf("Pending (%d):\n\n", len(items))

	if len(items) == 0 {
		fmt.Println("  No pending imports")
		return
	}

	fmt.Printf("  %-4s %-28s %-12s %s\n", "ID", "TITLE", "IMPORTED", "PLEX")
	fmt.Println("  " + strings.Repeat("-", 56))

	for _, dl := range items {
		title := dl.ReleaseName
		if len(title) > 28 {
			title = title[:25] + "..."
		}
		imported := "-"
		if dl.CompletedAt != nil {
			if t, err := time.Parse(time.RFC3339, *dl.CompletedAt); err == nil {
				imported = formatTimeAgo(t.Unix())
				// Warn if waiting too long
				if time.Since(t) > time.Hour {
					fmt.Printf("  %-4d %-28s %-12s %s\n", dl.ID, title, imported, "waiting")
					fmt.Printf("    ! Waiting >1hr - run 'arrgo verify %d' to check\n", dl.ID)
					continue
				}
			}
		}
		fmt.Printf("  %-4d %-28s %-12s %s\n", dl.ID, title, imported, "waiting")
	}
}

func printRecentImports(items []DownloadResponse) {
	fmt.Printf("Recent (last 24h): %d\n\n", len(items))

	if len(items) == 0 {
		fmt.Println("  No recent imports")
		return
	}

	fmt.Printf("  %-4s %-28s %-12s %s\n", "ID", "TITLE", "IMPORTED", "STATUS")
	fmt.Println("  " + strings.Repeat("-", 56))

	for _, dl := range items {
		title := dl.ReleaseName
		if len(title) > 28 {
			title = title[:25] + "..."
		}
		imported := "-"
		if dl.CompletedAt != nil {
			if t, err := time.Parse(time.RFC3339, *dl.CompletedAt); err == nil {
				imported = formatTimeAgo(t.Unix())
			}
		}
		fmt.Printf("  %-4d %-28s %-12s %s\n", dl.ID, title, imported, "done")
	}
}
