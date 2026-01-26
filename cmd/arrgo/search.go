package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmunix/arrgo/pkg/release"
)

// tvdbLookup searches TVDB and returns selected series info.
// Returns tvdbID, title, year, or 0, "", 0 if canceled or not found.
func tvdbLookup(client *Client, query string) (int64, string, int) {
	// Call server API to search TVDB
	results, err := client.TVDBSearch(query)
	if err != nil || len(results) == 0 {
		fmt.Println("No series found on TVDB")
		return 0, "", 0
	}

	if len(results) == 1 {
		r := results[0]
		fmt.Printf("Found: %s (%d) [TVDB:%d]\n", r.Name, r.Year, r.ID)
		return int64(r.ID), r.Name, r.Year
	}

	// Multiple results - prompt user
	fmt.Println("Multiple series found:")
	for i, r := range results {
		fmt.Printf("  %d. %s (%d)\n", i+1, r.Name, r.Year)
	}

	input := prompt(fmt.Sprintf("Select series [1-%d, n to cancel]: ", len(results)))
	if input == "n" || input == "N" || input == "" {
		return 0, "", 0
	}

	idx, _ := strconv.Atoi(input)
	if idx < 1 || idx > len(results) {
		return 0, "", 0
	}

	r := results[idx-1]
	return int64(r.ID), r.Name, r.Year
}

var searchCmd = &cobra.Command{
	Use:   "search [flags] <query>...",
	Short: "Search indexers for content",
	Long: `Search indexers for content.

Examples:
  arrgo search "The Matrix"
  arrgo search --verbose "The Matrix"
  arrgo search "The Matrix" --type movie --grab best`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSearchCmd,
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.Flags().BoolP("verbose", "v", false, "Show indexer, group, service")
	searchCmd.Flags().String("type", "", "Content type (movie or series)")
	searchCmd.Flags().String("profile", "", "Quality profile")
	searchCmd.Flags().String("grab", "", "Grab release: number or 'best'")
}

func runSearchCmd(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")
	verbose, _ := cmd.Flags().GetBool("verbose")
	contentType, _ := cmd.Flags().GetString("type")
	profile, _ := cmd.Flags().GetString("profile")
	grabFlag, _ := cmd.Flags().GetString("grab")

	client := NewClient(serverURL)

	// For series searches, do TVDB lookup first to capture the ID
	var tvdbID int64
	if contentType == "series" {
		tvdbID, _, _ = tvdbLookup(client, query)
		// If user canceled the TVDB selection, we still proceed with the search
		// The tvdbID will be 0 if canceled or not found
	}

	results, err := client.Search(query, contentType, profile)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if jsonOutput {
		printJSON(results)
		return nil
	}

	if len(results.Releases) == 0 {
		fmt.Println("No releases found")
		return nil
	}

	printSearchHumanCobra(query, results, verbose)

	// Handle grab
	var grabIndex int
	switch {
	case grabFlag == "best":
		grabIndex = 1
	case grabFlag != "":
		grabIndex, _ = strconv.Atoi(grabFlag)
	case !jsonOutput:
		// Interactive prompt
		input := prompt(fmt.Sprintf("\nGrab? [1-%d, n]: ", len(results.Releases)))
		if input == "" || input == "n" || input == "N" {
			return nil
		}
		grabIndex, _ = strconv.Atoi(input)
	}

	if grabIndex < 1 || grabIndex > len(results.Releases) {
		return nil
	}

	// Grab the selected release
	selected := results.Releases[grabIndex-1]
	grabRelease(client, selected, contentType, profile, tvdbID)
	return nil
}

func printSearchHumanCobra(query string, r *SearchResponse, verbose bool) {
	fmt.Printf("Found %d releases for %q:\n\n", len(r.Releases), query)
	fmt.Printf("  # │ %-42s │ %8s │ %5s\n", "RELEASE", "SIZE", "SCORE")
	fmt.Println("────┼────────────────────────────────────────────┼──────────┼───────")

	for i, rel := range r.Releases {
		title := rel.Title
		if len(title) > 42 {
			title = title[:39] + "..."
		}
		fmt.Printf(" %2d │ %-42s │ %8s │ %5d\n",
			i+1, title, formatSize(rel.Size), rel.Score)

		// Parse release to get quality info for badges
		info := release.Parse(rel.Title)
		badges := buildBadges(info)
		if badges != "" {
			fmt.Printf("    │ %s\n", badges)
		}

		// Verbose mode: show indexer, group, and service
		if verbose {
			var parts []string
			if rel.Indexer != "" {
				parts = append(parts, "Indexer: "+rel.Indexer)
			}
			if info.Group != "" {
				parts = append(parts, "Group: "+info.Group)
			}
			if info.Service != "" {
				parts = append(parts, "Service: "+info.Service)
			}
			if len(parts) > 0 {
				fmt.Printf("    │ %s\n", strings.Join(parts, "  "))
			}
		}
	}

	if len(r.Errors) > 0 {
		fmt.Printf("\nWarnings: %s\n", strings.Join(r.Errors, ", "))
	}
}
