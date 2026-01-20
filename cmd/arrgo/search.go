package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmunix/arrgo/pkg/release"
)

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
	if grabFlag == "best" {
		grabIndex = 1
	} else if grabFlag != "" {
		grabIndex, _ = strconv.Atoi(grabFlag)
	} else if !jsonOutput {
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
	grabRelease(client, selected, contentType, profile)
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
