package main

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vmunix/arrgo/pkg/release"
)

var importCmd = &cobra.Command{
	Use:   "import [download_id]",
	Short: "Import downloaded content into the library",
	Long: `Import downloaded content into the library.

Modes:
  Tracked:  arrgo import <download_id>     - Import a tracked download
  Manual:   arrgo import --manual <path>   - Import from a file path

Subcommands:
  list      Show pending imports and recent completions

Examples:
  arrgo import 42
  arrgo import --manual "/downloads/Movie.Name.2024.1080p.WEB-DL.mkv"
  arrgo import --manual "/downloads/Show.S01E05.720p.HDTV.mkv" --dry-run
  arrgo import list
  arrgo import list --pending`,
	Args: cobra.MaximumNArgs(1),
	RunE: runImportCmd,
}

var importListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show pending imports and recent completions",
	RunE:  runImportListCmd,
}

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().String("manual", "", "Path to file for manual import")
	importCmd.Flags().Bool("dry-run", false, "Preview import without making changes")

	// Add list subcommand
	importCmd.AddCommand(importListCmd)
	importListCmd.Flags().Bool("pending", false, "Only show pending imports")
	importListCmd.Flags().Bool("recent", false, "Only show recent completions")
}

func runImportCmd(cmd *cobra.Command, args []string) error {
	manualPath, _ := cmd.Flags().GetString("manual")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	// Determine mode
	if manualPath != "" {
		return runManualImport(manualPath, dryRun)
	}

	if len(args) == 0 {
		return fmt.Errorf("either provide a download ID or use --manual <path>\n\nUse 'arrgo import list' to see pending imports")
	}

	downloadID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid download ID: %s", args[0])
	}

	return runTrackedImport(downloadID, dryRun)
}

func runManualImport(path string, dryRun bool) error {
	// Extract filename for parsing
	filename := filepath.Base(path)
	info := release.Parse(filename)

	// Determine content type based on S##E## presence
	contentType := "movie"
	if info.Season > 0 || info.Episode > 0 {
		contentType = "series"
	}

	// Build quality string
	quality := info.Resolution.String()
	if quality == valueUnknown {
		quality = ""
	}

	if dryRun {
		return printDryRunPreview(path, info, contentType, quality)
	}

	// Build request
	req := &ImportRequest{
		Path:    path,
		Title:   info.Title,
		Year:    info.Year,
		Type:    contentType,
		Quality: quality,
	}
	if info.Season > 0 {
		req.Season = &info.Season
	}
	if info.Episode > 0 {
		req.Episode = &info.Episode
	}

	client := NewClient(serverURL)
	resp, err := client.Import(req)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	if jsonOutput {
		printJSON(resp)
		return nil
	}

	printImportResult(resp)
	return nil
}

func runTrackedImport(downloadID int64, dryRun bool) error {
	if dryRun {
		fmt.Println("Dry-run mode for tracked imports requires the server.")
		fmt.Println("Use --dry-run with --manual to preview release parsing locally.")
		return nil
	}

	req := &ImportRequest{
		DownloadID: &downloadID,
	}

	client := NewClient(serverURL)
	resp, err := client.Import(req)
	if err != nil {
		return fmt.Errorf("import failed: %w", err)
	}

	if jsonOutput {
		printJSON(resp)
		return nil
	}

	printImportResult(resp)
	return nil
}

func printDryRunPreview(path string, info *release.Info, contentType, quality string) error {
	if jsonOutput {
		preview := map[string]any{
			"dry_run":      true,
			"source_path":  path,
			"parsed_title": info.Title,
			"year":         info.Year,
			"type":         contentType,
			"quality":      quality,
		}
		if info.Season > 0 {
			preview["season"] = info.Season
		}
		if info.Episode > 0 {
			preview["episode"] = info.Episode
		}
		printJSON(preview)
		return nil
	}

	fmt.Println("Dry-run preview (no changes made):")
	fmt.Println()
	fmt.Printf("  Source: %s\n", path)
	fmt.Printf("  Title:  %s\n", info.Title)
	if info.Year > 0 {
		fmt.Printf("  Year:   %d\n", info.Year)
	}
	fmt.Printf("  Type:   %s\n", contentType)
	if quality != "" {
		fmt.Printf("  Quality: %s\n", quality)
	}
	if info.Season > 0 {
		fmt.Printf("  Season: %d\n", info.Season)
	}
	if info.Episode > 0 {
		fmt.Printf("  Episode: %d\n", info.Episode)
	}

	// Show badges
	badges := buildBadges(info)
	if badges != "" {
		fmt.Printf("  Badges: %s\n", badges)
	}

	return nil
}

func printImportResult(resp *ImportResponse) {
	fmt.Println("Import successful:")
	fmt.Println()
	fmt.Printf("  File ID:    %d\n", resp.FileID)
	fmt.Printf("  Content ID: %d\n", resp.ContentID)
	fmt.Printf("  Source:     %s\n", resp.SourcePath)
	fmt.Printf("  Dest:       %s\n", resp.DestPath)
	fmt.Printf("  Size:       %s\n", formatSize(resp.SizeBytes))

	if resp.PlexNotified {
		fmt.Println("  Plex:       notified")
	} else {
		fmt.Println("  Plex:       not notified")
	}
}

// --- import list subcommand ---

func runImportListCmd(cmd *cobra.Command, args []string) error {
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
	for i := range downloads.Items {
		dl := &downloads.Items[i]
		switch dl.Status {
		case "imported":
			pending = append(pending, *dl)
		case "cleaned":
			// Only include if completed within last 24h
			if dl.CompletedAt != nil {
				if t, err := time.Parse(time.RFC3339, *dl.CompletedAt); err == nil {
					if time.Since(t) < 24*time.Hour {
						recent = append(recent, *dl)
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

	for i := range items {
		dl := &items[i]
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

	for i := range items {
		dl := &items[i]
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
