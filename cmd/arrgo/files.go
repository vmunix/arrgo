package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// FileResponse matches the API response for a file.
type FileResponse struct {
	ID        int64     `json:"id"`
	ContentID int64     `json:"content_id"`
	EpisodeID *int64    `json:"episode_id,omitempty"`
	Path      string    `json:"path"`
	SizeBytes int64     `json:"size_bytes"`
	Quality   string    `json:"quality"`
	Source    string    `json:"source"`
	AddedAt   time.Time `json:"added_at"`
}

// ListFilesResponse matches the API response for listing files.
type ListFilesResponse struct {
	Items []FileResponse `json:"items"`
	Total int            `json:"total"`
}

func init() {
	filesCmd := &cobra.Command{
		Use:   "files [content-id]",
		Short: "List tracked files",
		Long:  "Lists files tracked in the library. Optionally filter by content ID.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runFilesCmd,
	}

	rootCmd.AddCommand(filesCmd)
}

func runFilesCmd(cmd *cobra.Command, args []string) error {
	var contentID *int64
	if len(args) > 0 {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid content ID: %s", args[0])
		}
		contentID = &id
	}

	client := NewClient(serverURL)
	files, err := client.Files(contentID)
	if err != nil {
		return fmt.Errorf("failed to fetch files: %w", err)
	}

	if jsonOutput {
		printJSON(files)
		return nil
	}

	if len(files.Items) == 0 {
		if contentID != nil {
			fmt.Printf("No files for content %d.\n", *contentID)
		} else {
			fmt.Println("No files tracked.")
		}
		return nil
	}

	if contentID != nil {
		printFilesForContent(files, *contentID)
	} else {
		printFilesAll(files)
	}
	return nil
}

func printFilesAll(f *ListFilesResponse) {
	fmt.Printf("Files (%d):\n\n", f.Total)
	fmt.Printf("  %-4s %-8s %-45s %-10s %s\n", "ID", "CONTENT", "PATH", "SIZE", "QUALITY")
	fmt.Println("  " + strings.Repeat("-", 80))

	for i := range f.Items {
		file := &f.Items[i]
		path := truncatePath(file.Path, 45)
		fmt.Printf("  %-4d %-8d %-45s %-10s %s\n",
			file.ID,
			file.ContentID,
			path,
			formatSize(file.SizeBytes),
			file.Quality)
	}
}

func printFilesForContent(f *ListFilesResponse, contentID int64) {
	fmt.Printf("Files for content %d (%d):\n\n", contentID, f.Total)
	fmt.Printf("  %-4s %-55s %-10s %s\n", "ID", "PATH", "SIZE", "QUALITY")
	fmt.Println("  " + strings.Repeat("-", 80))

	for i := range f.Items {
		file := &f.Items[i]
		path := truncatePath(file.Path, 55)
		fmt.Printf("  %-4d %-55s %-10s %s\n",
			file.ID,
			path,
			formatSize(file.SizeBytes),
			file.Quality)
	}
}

// truncatePath shortens a path for display, keeping the end visible.
// If the path is longer than maxLen, it will be shown as ".../<end>".
func truncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	// Show as much of the end as possible with "..." prefix
	if maxLen <= 3 {
		return path[:maxLen]
	}
	return "..." + path[len(path)-(maxLen-3):]
}
