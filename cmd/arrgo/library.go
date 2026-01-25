package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// LibraryContentResponse matches the API response for content items.
type LibraryContentResponse struct {
	ID             int64     `json:"id"`
	Type           string    `json:"type"`
	TMDBID         *int64    `json:"tmdb_id,omitempty"`
	TVDBID         *int64    `json:"tvdb_id,omitempty"`
	Title          string    `json:"title"`
	Year           int       `json:"year"`
	Status         string    `json:"status"`
	QualityProfile string    `json:"quality_profile"`
	RootPath       string    `json:"root_path"`
	AddedAt        time.Time `json:"added_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ListLibraryResponse matches the API response for listing content.
type ListLibraryResponse struct {
	Items  []LibraryContentResponse `json:"items"`
	Total  int                      `json:"total"`
	Limit  int                      `json:"limit"`
	Offset int                      `json:"offset"`
}

// LibraryCheckItem matches the API response for a single check item.
type LibraryCheckItem struct {
	ID          int64    `json:"id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Year        int      `json:"year"`
	Status      string   `json:"status"`
	FileCount   int      `json:"file_count"`
	Files       []string `json:"files,omitempty"`
	FileExists  bool     `json:"file_exists"`
	FileMissing []string `json:"file_missing,omitempty"`
	InPlex      bool     `json:"in_plex"`
	PlexTitle   string   `json:"plex_title,omitempty"`
	Issues      []string `json:"issues,omitempty"`
}

// LibraryCheckResponse matches the API response for library check.
type LibraryCheckResponse struct {
	Items      []LibraryCheckItem `json:"items"`
	Total      int                `json:"total"`
	Healthy    int                `json:"healthy"`
	WithIssues int                `json:"with_issues"`
}

func init() {
	libraryCmd := &cobra.Command{
		Use:   "library",
		Short: "Manage library content",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List content in library",
		RunE:  runLibraryList,
	}

	listCmd.Flags().StringP("type", "t", "", "Filter by type (movie, series)")
	listCmd.Flags().StringP("status", "s", "", "Filter by status (wanted, available, missing)")
	listCmd.Flags().IntP("limit", "l", 50, "Maximum number of items to return")

	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Verify files exist and Plex awareness",
		Long:  "Checks each item in the library to verify files exist at expected paths and are present in Plex.",
		RunE:  runLibraryCheck,
	}

	checkCmd.Flags().StringP("type", "t", "", "Filter by type (movie, series)")
	checkCmd.Flags().StringP("status", "s", "", "Filter by status (wanted, available, missing)")
	checkCmd.Flags().IntP("limit", "l", 100, "Maximum number of items to check")
	checkCmd.Flags().Bool("issues-only", false, "Only show items with issues")

	deleteCmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete content from library",
		Long:  "Removes content and associated file records from the library. Does not delete actual files on disk.",
		Args:  cobra.ExactArgs(1),
		RunE:  runLibraryDelete,
	}

	addCmd := &cobra.Command{
		Use:   "add",
		Short: "Add content to library",
		Long:  "Adds a new movie or series to the library for tracking.",
		RunE:  runLibraryAdd,
	}

	addCmd.Flags().String("title", "", "Title of the content (required)")
	addCmd.Flags().Int("year", 0, "Release year (required)")
	addCmd.Flags().String("type", "", "Content type: movie or series (required)")
	addCmd.Flags().Int64("tmdb-id", 0, "The Movie Database ID")
	addCmd.Flags().Int64("tvdb-id", 0, "TheTVDB ID")
	addCmd.Flags().String("quality", "", "Quality profile (e.g., hd, uhd)")
	addCmd.Flags().Bool("json", false, "Output as JSON")

	_ = addCmd.MarkFlagRequired("title")
	_ = addCmd.MarkFlagRequired("year")
	_ = addCmd.MarkFlagRequired("type")

	libraryCmd.AddCommand(listCmd)
	libraryCmd.AddCommand(checkCmd)
	libraryCmd.AddCommand(deleteCmd)
	libraryCmd.AddCommand(addCmd)
	rootCmd.AddCommand(libraryCmd)
}

func runLibraryList(cmd *cobra.Command, args []string) error {
	typeFilter, _ := cmd.Flags().GetString("type")
	statusFilter, _ := cmd.Flags().GetString("status")
	limit, _ := cmd.Flags().GetInt("limit")

	// Build query params
	params := url.Values{}
	if typeFilter != "" {
		params.Set("type", typeFilter)
	}
	if statusFilter != "" {
		params.Set("status", statusFilter)
	}
	params.Set("limit", fmt.Sprintf("%d", limit))

	urlStr := fmt.Sprintf("%s/api/v1/content", serverURL)
	if len(params) > 0 {
		urlStr += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var data ListLibraryResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(data.Items) == 0 {
		fmt.Println("No content in library.")
		return nil
	}

	printLibraryList(&data)
	return nil
}

func printLibraryList(data *ListLibraryResponse) {
	fmt.Printf("Library (%d items):\n\n", data.Total)
	fmt.Printf("  %-4s %-8s %-40s %-6s %-10s %s\n", "ID", "TYPE", "TITLE", "YEAR", "STATUS", "QUALITY")
	fmt.Println("  " + strings.Repeat("-", 90))

	for i := range data.Items {
		item := &data.Items[i]
		title := item.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}

		fmt.Printf("  %-4d %-8s %-40s %-6d %-10s %s\n",
			item.ID,
			item.Type,
			title,
			item.Year,
			item.Status,
			item.QualityProfile)
	}

	if data.Total > len(data.Items) {
		fmt.Printf("\n  Showing %d of %d items. Use --limit to see more.\n", len(data.Items), data.Total)
	}
}

func runLibraryCheck(cmd *cobra.Command, args []string) error {
	typeFilter, _ := cmd.Flags().GetString("type")
	statusFilter, _ := cmd.Flags().GetString("status")
	limit, _ := cmd.Flags().GetInt("limit")
	issuesOnly, _ := cmd.Flags().GetBool("issues-only")

	// Build query params
	params := url.Values{}
	if typeFilter != "" {
		params.Set("type", typeFilter)
	}
	if statusFilter != "" {
		params.Set("status", statusFilter)
	}
	params.Set("limit", fmt.Sprintf("%d", limit))

	urlStr := fmt.Sprintf("%s/api/v1/library/check", serverURL)
	if len(params) > 0 {
		urlStr += "?" + params.Encode()
	}

	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var data LibraryCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(data.Items) == 0 {
		fmt.Println("No content in library.")
		return nil
	}

	printLibraryCheck(&data, issuesOnly)
	return nil
}

func printLibraryCheck(data *LibraryCheckResponse, issuesOnly bool) {
	fmt.Printf("Library Check (%d items, %d healthy, %d with issues):\n\n", data.Total, data.Healthy, data.WithIssues)

	for i := range data.Items {
		item := &data.Items[i]

		// Skip healthy items if issues-only flag is set
		if issuesOnly && len(item.Issues) == 0 {
			continue
		}

		// Status indicators
		fileStatus := "✓"
		if !item.FileExists {
			fileStatus = "✗"
		}
		plexStatus := "✓"
		if !item.InPlex {
			plexStatus = "✗"
		}

		title := item.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}

		fmt.Printf("  [%d] %s (%d) - %s\n", item.ID, title, item.Year, item.Status)
		fmt.Printf("      Files: %s (%d)  Plex: %s", fileStatus, item.FileCount, plexStatus)
		if item.PlexTitle != "" && item.PlexTitle != item.Title {
			fmt.Printf(" (%s)", item.PlexTitle)
		}
		fmt.Println()

		if len(item.Issues) > 0 {
			fmt.Println("      Issues:")
			for _, issue := range item.Issues {
				fmt.Printf("        - %s\n", issue)
			}
		}

		fmt.Println()
	}

	// Summary
	if data.WithIssues > 0 {
		fmt.Printf("Summary: %d/%d items have issues\n", data.WithIssues, data.Total)
	} else {
		fmt.Println("All items healthy!")
	}
}

func runLibraryDelete(cmd *cobra.Command, args []string) error {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid ID: %s", args[0])
	}

	// First get the content to show what we're deleting
	urlStr := fmt.Sprintf("%s/api/v1/content/%d", serverURL, id)

	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("content ID %d not found", id)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var content LibraryContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	// Now delete
	req, err = http.NewRequest(http.MethodDelete, urlStr, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("delete failed: server returned %d", resp.StatusCode)
	}

	fmt.Printf("Deleted: %s (%d)\n", content.Title, content.Year)
	return nil
}

// addContentRequest is the request body for POST /api/v1/content.
type addContentRequest struct {
	Type           string `json:"type"`
	TMDBID         *int64 `json:"tmdb_id,omitempty"`
	TVDBID         *int64 `json:"tvdb_id,omitempty"`
	Title          string `json:"title"`
	Year           int    `json:"year"`
	QualityProfile string `json:"quality_profile,omitempty"`
}

func runLibraryAdd(cmd *cobra.Command, args []string) error {
	title, _ := cmd.Flags().GetString("title")
	year, _ := cmd.Flags().GetInt("year")
	contentType, _ := cmd.Flags().GetString("type")
	tmdbID, _ := cmd.Flags().GetInt64("tmdb-id")
	tvdbID, _ := cmd.Flags().GetInt64("tvdb-id")
	quality, _ := cmd.Flags().GetString("quality")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	// Validate type
	if contentType != "movie" && contentType != "series" {
		return fmt.Errorf("--type must be 'movie' or 'series', got: %s", contentType)
	}

	// Build request body
	reqBody := addContentRequest{
		Type:           contentType,
		Title:          title,
		Year:           year,
		QualityProfile: quality,
	}

	if tmdbID != 0 {
		reqBody.TMDBID = &tmdbID
	}
	if tvdbID != 0 {
		reqBody.TVDBID = &tvdbID
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	urlStr := fmt.Sprintf("%s/api/v1/content", serverURL)
	req, err := http.NewRequest(http.MethodPost, urlStr, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		// Try to read error message
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return fmt.Errorf("server error: %s", errResp.Error)
		}
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var content LibraryContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&content); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(content)
	}

	fmt.Printf("Added: %s (%d) [ID: %d, status: %s]\n", content.Title, content.Year, content.ID, content.Status)
	return nil
}
