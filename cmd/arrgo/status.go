package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [download-id]",
	Short: "System status and verification",
	Long: `Show system status and verify download states against live systems.

Without arguments, shows system dashboard (connections, downloads, library).
With a download ID, verifies that specific download against SABnzbd/filesystem/Plex.

Examples:
  arrgo status                # Show system dashboard
  arrgo status --verify       # Dashboard + run verification on all downloads
  arrgo status 42             # Verify specific download #42`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStatusCmd,
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().Bool("verify", false, "Run verification on all downloads")
}

func runStatusCmd(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)
	runVerify, _ := cmd.Flags().GetBool("verify")

	// If a download ID is provided, verify that specific download
	if len(args) > 0 {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid download ID: %s", args[0])
		}
		return runVerifyDownload(client, &id)
	}

	// Get dashboard
	dash, err := client.Dashboard()
	if err != nil {
		return fmt.Errorf("status check failed: %w", err)
	}

	if jsonOutput {
		if runVerify {
			// Combine dashboard and verify results
			verify, err := client.Verify(nil)
			if err != nil {
				return fmt.Errorf("verify failed: %w", err)
			}
			combined := map[string]any{
				"dashboard": dash,
				"verify":    verify,
			}
			printJSON(combined)
		} else {
			printJSON(dash)
		}
		return nil
	}

	printDashboard(serverURL, dash)

	// If --verify flag or there are stuck downloads, run verification
	// (failed downloads are shown in verify output but don't trigger auto-verify)
	if runVerify || dash.Stuck.Count > 0 {
		fmt.Println()
		return runVerifyDownload(client, nil)
	}

	return nil
}

func runVerifyDownload(client *Client, id *int64) error {
	result, err := client.Verify(id)
	if err != nil {
		return fmt.Errorf("verify failed: %w", err)
	}

	if jsonOutput {
		printJSON(result)
		return nil
	}

	printVerifyResult(result)
	return nil
}

func printDashboard(server string, d *DashboardResponse) {
	// Header
	plexStatus := "disconnected"
	if d.Connections.Plex {
		plexStatus = "connected"
	}
	sabStatus := "disconnected"
	if d.Connections.SABnzbd {
		sabStatus = "connected"
	}

	fmt.Printf("arrgo v%s | Server: %s | Plex: %s | SABnzbd: %s\n\n",
		d.Version, server, plexStatus, sabStatus)

	// Downloads
	fmt.Println("Downloads")
	fmt.Printf("  Queued:       %d\n", d.Downloads.Queued)
	fmt.Printf("  Downloading:  %d\n", d.Downloads.Downloading)
	fmt.Printf("  Completed:    %d\n", d.Downloads.Completed)
	fmt.Printf("  Importing:    %d\n", d.Downloads.Importing)
	fmt.Printf("  Imported:     %d  (awaiting Plex verification)\n", d.Downloads.Imported)
	fmt.Println()

	// Library
	fmt.Println("Library")
	fmt.Printf("  Movies:     %d tracked\n", d.Library.Movies)
	fmt.Printf("  Series:     %d tracked\n", d.Library.Series)
	fmt.Println()

	// Problems summary
	if d.Stuck.Count > 0 {
		fmt.Printf("Problems: %d stuck downloads (running verification...)\n", d.Stuck.Count)
	}
	if d.Downloads.Failed > 0 {
		fmt.Printf("Failed: %d downloads (use 'arrgo downloads -s failed' to see)\n", d.Downloads.Failed)
	}
}

func printVerifyResult(r *VerifyResponse) {
	fmt.Printf("Verification (%d downloads checked):\n\n", r.Checked)

	// Connection status
	plexStatus := "ok"
	if !r.Connections.Plex {
		plexStatus = "FAIL " + r.Connections.PlexErr
	}
	sabStatus := "ok"
	if !r.Connections.SABnzbd {
		sabStatus = "FAIL " + r.Connections.SABErr
	}
	fmt.Printf("  SABnzbd: %s\n", sabStatus)
	fmt.Printf("  Plex:    %s\n", plexStatus)
	fmt.Printf("  Passed:  %d/%d\n", r.Passed, r.Checked)
	fmt.Println()

	if len(r.Problems) == 0 {
		fmt.Println("No problems detected.")
		return
	}

	fmt.Printf("Problems (%d):\n\n", len(r.Problems))

	for i := range r.Problems {
		p := &r.Problems[i]
		fmt.Printf("  ID %d | %s | %s\n", p.DownloadID, p.Status, p.Title)
		fmt.Printf("    State: %s (%s)\n", p.Status, p.Since)
		fmt.Printf("    Issue: %s\n", p.Issue)
		for _, check := range p.Checks {
			fmt.Printf("    Check: %s\n", check)
		}
		fmt.Printf("    Likely: %s\n", p.Likely)
		fmt.Printf("    Fix: %s\n", strings.Join(p.Fixes, "\n         "))
		fmt.Println()
	}

	fmt.Printf("%d problems found. Run suggested commands to resolve.\n", len(r.Problems))
}
