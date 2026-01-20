package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var verifyCmd = &cobra.Command{
	Use:   "verify [download-id]",
	Short: "Verify download states against live systems",
	Long:  "Compare what arrgo thinks vs reality (SABnzbd, filesystem, Plex)",
	RunE:  runVerifyCmd,
}

func init() {
	rootCmd.AddCommand(verifyCmd)
}

func runVerifyCmd(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)

	var id *int64
	if len(args) > 0 {
		parsed, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid download ID: %s", args[0])
		}
		id = &parsed
	}

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

func printVerifyResult(r *VerifyResponse) {
	fmt.Printf("Checking %d downloads...\n\n", r.Checked)

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

	for _, p := range r.Problems {
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

	fmt.Printf("%d problems found. Run suggested commands or 'arrgo verify --help' for options.\n", len(r.Problems))
}
