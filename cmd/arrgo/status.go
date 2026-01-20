package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "System status (health, disk, queue summary)",
	RunE:  runStatusCmd,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatusCmd(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)
	dash, err := client.Dashboard()
	if err != nil {
		return fmt.Errorf("status check failed: %w", err)
	}

	if jsonOutput {
		printJSON(dash)
		return nil
	}

	printDashboard(serverURL, dash)
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
	fmt.Printf("  Imported:     %d  (awaiting Plex verification)\n", d.Downloads.Imported)
	fmt.Println()

	// Library
	fmt.Println("Library")
	fmt.Printf("  Movies:     %d tracked\n", d.Library.Movies)
	fmt.Printf("  Series:     %d tracked\n", d.Library.Series)
	fmt.Println()

	// Problems
	if d.Stuck.Count > 0 || d.Downloads.Failed > 0 {
		problems := d.Stuck.Count + d.Downloads.Failed
		fmt.Printf("Problems: %d detected\n", problems)
		fmt.Println("  -> Run 'arrgo verify' for details")
	}
}
