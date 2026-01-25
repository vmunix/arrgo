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

// LibraryEpisodeStats matches the API response for series episode statistics.
type LibraryEpisodeStats struct {
	TotalEpisodes     int `json:"total_episodes"`
	AvailableEpisodes int `json:"available_episodes"`
	SeasonCount       int `json:"season_count"`
}

// LibraryContentResponse matches the API response for content items.
type LibraryContentResponse struct {
	ID             int64                `json:"id"`
	Type           string               `json:"type"`
	TMDBID         *int64               `json:"tmdb_id,omitempty"`
	TVDBID         *int64               `json:"tvdb_id,omitempty"`
	Title          string               `json:"title"`
	Year           int                  `json:"year"`
	Status         string               `json:"status"`
	QualityProfile string               `json:"quality_profile"`
	RootPath       string               `json:"root_path"`
	AddedAt        time.Time            `json:"added_at"`
	UpdatedAt      time.Time            `json:"updated_at"`
	EpisodeStats   *LibraryEpisodeStats `json:"episode_stats,omitempty"`
}

// EpisodeResponse matches the API response for episodes.
type EpisodeResponse struct {
	ID        int64      `json:"id"`
	ContentID int64      `json:"content_id"`
	Season    int        `json:"season"`
	Episode   int        `json:"episode"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	AirDate   *time.Time `json:"air_date,omitempty"`
}

// ListEpisodesResponse matches the API response for listing episodes.
type ListEpisodesResponse struct {
	Items []EpisodeResponse `json:"items"`
	Total int               `json:"total"`
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

	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Show content details",
		Long:  "Shows detailed information about a content item. For series, includes episode list grouped by season.",
		Args:  cobra.ExactArgs(1),
		RunE:  runLibraryShow,
	}

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

	_ = addCmd.MarkFlagRequired("title")
	_ = addCmd.MarkFlagRequired("year")
	_ = addCmd.MarkFlagRequired("type")

	importCmd := &cobra.Command{
		Use:   "import",
		Short: "Import existing media library",
		Long:  "Import untracked items from Plex into arrgo for tracking.",
		RunE:  runLibraryImport,
	}

	importCmd.Flags().String("from-plex", "", "Import from Plex library by name (required)")
	importCmd.Flags().String("quality", "", "Override quality profile for all imports")
	importCmd.Flags().Bool("dry-run", false, "Preview import without making changes")

	libraryCmd.AddCommand(listCmd)
	libraryCmd.AddCommand(showCmd)
	libraryCmd.AddCommand(checkCmd)
	libraryCmd.AddCommand(deleteCmd)
	libraryCmd.AddCommand(addCmd)
	libraryCmd.AddCommand(importCmd)
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

func runLibraryShow(cmd *cobra.Command, args []string) error {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid ID: %s", args[0])
	}

	// Get content details
	contentURL := fmt.Sprintf("%s/api/v1/content/%d", serverURL, id)
	req, err := http.NewRequest(http.MethodGet, contentURL, nil)
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
		return fmt.Errorf("decode content: %w", err)
	}

	// For series, also fetch episodes
	var episodes *ListEpisodesResponse
	if content.Type == contentTypeSeries {
		episodesURL := fmt.Sprintf("%s/api/v1/content/%d/episodes", serverURL, id)
		req, err = http.NewRequest(http.MethodGet, episodesURL, nil)
		if err != nil {
			return fmt.Errorf("create episodes request: %w", err)
		}

		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("episodes request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			episodes = &ListEpisodesResponse{}
			if err := json.NewDecoder(resp.Body).Decode(episodes); err != nil {
				return fmt.Errorf("decode episodes: %w", err)
			}
		}
	}

	if jsonOutput {
		// For JSON, include episodes in response
		output := struct {
			LibraryContentResponse
			Episodes []EpisodeResponse `json:"episodes,omitempty"`
		}{
			LibraryContentResponse: content,
		}
		if episodes != nil {
			output.Episodes = episodes.Items
		}
		printJSON(output)
		return nil
	}

	printLibraryShow(&content, episodes)
	return nil
}

func printLibraryShow(content *LibraryContentResponse, episodes *ListEpisodesResponse) {
	fmt.Printf("%s #%d\n\n", strings.ToUpper(content.Type), content.ID)

	fmt.Printf("  Title:    %s\n", content.Title)
	if content.Year > 0 {
		fmt.Printf("  Year:     %d\n", content.Year)
	}
	fmt.Printf("  Status:   %s\n", content.Status)
	fmt.Printf("  Quality:  %s\n", content.QualityProfile)

	if content.TMDBID != nil {
		fmt.Printf("  TMDB ID:  %d\n", *content.TMDBID)
	}
	if content.TVDBID != nil {
		fmt.Printf("  TVDB ID:  %d\n", *content.TVDBID)
	}

	fmt.Printf("  Added:    %s\n", content.AddedAt.Format("2006-01-02 15:04"))

	// For series, show episode breakdown
	if content.Type == contentTypeSeries && episodes != nil && len(episodes.Items) > 0 {
		fmt.Printf("\n  Episodes (%d total):\n", episodes.Total)

		// Group episodes by season
		seasons := make(map[int][]EpisodeResponse)
		for _, ep := range episodes.Items {
			seasons[ep.Season] = append(seasons[ep.Season], ep)
		}

		// Get sorted season numbers
		seasonNums := make([]int, 0, len(seasons))
		for s := range seasons {
			seasonNums = append(seasonNums, s)
		}
		// Sort seasons
		for i := 0; i < len(seasonNums)-1; i++ {
			for j := i + 1; j < len(seasonNums); j++ {
				if seasonNums[i] > seasonNums[j] {
					seasonNums[i], seasonNums[j] = seasonNums[j], seasonNums[i]
				}
			}
		}

		for _, seasonNum := range seasonNums {
			eps := seasons[seasonNum]
			// Count statuses
			available := 0
			wanted := 0
			for _, ep := range eps {
				if ep.Status == "available" {
					available++
				} else if ep.Status == "wanted" {
					wanted++
				}
			}

			var statusStr string
			switch {
			case available == len(eps):
				statusStr = "all available"
			case wanted == len(eps):
				statusStr = "all wanted"
			default:
				statusStr = fmt.Sprintf("%d available, %d wanted", available, wanted)
			}

			fmt.Printf("\n    Season %d (%d episodes, %s):\n", seasonNum, len(eps), statusStr)
			fmt.Printf("      %-4s %-40s %s\n", "EP", "TITLE", "STATUS")
			fmt.Println("      " + strings.Repeat("-", 55))

			for _, ep := range eps {
				title := ep.Title
				if len(title) > 40 {
					title = title[:37] + "..."
				}
				fmt.Printf("      %-4d %-40s %s\n", ep.Episode, title, ep.Status)
			}
		}
	} else if content.Type == contentTypeSeries {
		fmt.Println("\n  No episodes tracked yet.")
	}
}

func printLibraryList(data *ListLibraryResponse) {
	fmt.Printf("Library (%d items):\n\n", data.Total)
	fmt.Printf("  %-4s %-8s %-40s %-6s %-18s %s\n", "ID", "TYPE", "TITLE", "YEAR", "STATUS", "QUALITY")
	fmt.Println("  " + strings.Repeat("-", 98))

	for i := range data.Items {
		item := &data.Items[i]
		title := item.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}

		// Format status - for series, include episode count
		status := item.Status
		if item.Type == contentTypeSeries && item.EpisodeStats != nil {
			status = fmt.Sprintf("%s (%d/%d)",
				item.Status,
				item.EpisodeStats.AvailableEpisodes,
				item.EpisodeStats.TotalEpisodes)
		}

		// Format year - show "-" if 0
		yearStr := fmt.Sprintf("%d", item.Year)
		if item.Year == 0 {
			yearStr = "-"
		}

		fmt.Printf("  %-4d %-8s %-40s %-6s %-18s %s\n",
			item.ID,
			item.Type,
			title,
			yearStr,
			status,
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

func runLibraryImport(cmd *cobra.Command, args []string) error {
	plexLibrary, _ := cmd.Flags().GetString("from-plex")
	quality, _ := cmd.Flags().GetString("quality")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if plexLibrary == "" {
		return fmt.Errorf("--from-plex is required")
	}

	client := NewClient(serverURL)
	resp, err := client.LibraryImport(&LibraryImportRequest{
		Source:          "plex",
		Library:         plexLibrary,
		QualityOverride: quality,
		DryRun:          dryRun,
	})
	if err != nil {
		return err
	}

	if jsonOutput {
		printJSON(resp)
		return nil
	}

	printLibraryImport(resp, plexLibrary, dryRun)
	return nil
}

func printLibraryImport(r *LibraryImportResponse, library string, dryRun bool) {
	action := "Importing"
	if dryRun {
		action = "Would import"
	}
	fmt.Printf("%s from Plex library %q...\n\n", action, library)

	for _, item := range r.Imported {
		quality := item.Quality
		if quality == "" {
			quality = "unknown"
		}
		fmt.Printf("  + %s (%d) - %s\n", item.Title, item.Year, quality)
	}
	for _, item := range r.Skipped {
		fmt.Printf("  - %s (%d) - %s\n", item.Title, item.Year, item.Reason)
	}
	for _, item := range r.Errors {
		fmt.Printf("  ! %s (%d) - %s\n", item.Title, item.Year, item.Error)
	}

	fmt.Println()
	if dryRun {
		fmt.Printf("Would import: %d new, %d skipped, %d errors\n",
			r.Summary.Imported, r.Summary.Skipped, r.Summary.Errors)
	} else {
		fmt.Printf("Imported: %d new, %d skipped, %d errors\n",
			r.Summary.Imported, r.Summary.Skipped, r.Summary.Errors)
	}
}
