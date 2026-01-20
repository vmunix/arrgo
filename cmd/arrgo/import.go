package main

import (
	"fmt"
	"path/filepath"
	"strconv"

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

Examples:
  arrgo import 42
  arrgo import --manual "/downloads/Movie.Name.2024.1080p.WEB-DL.mkv"
  arrgo import --manual "/downloads/Show.S01E05.720p.HDTV.mkv" --dry-run`,
	Args: cobra.MaximumNArgs(1),
	RunE: runImportCmd,
}

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().String("manual", "", "Path to file for manual import")
	importCmd.Flags().Bool("dry-run", false, "Preview import without making changes")
}

func runImportCmd(cmd *cobra.Command, args []string) error {
	manualPath, _ := cmd.Flags().GetString("manual")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	// Determine mode
	if manualPath != "" {
		return runManualImport(manualPath, dryRun)
	}

	if len(args) == 0 {
		return fmt.Errorf("either provide a download ID or use --manual <path>")
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
	if quality == "unknown" {
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
