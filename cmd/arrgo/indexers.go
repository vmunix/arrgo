package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var indexersCmd = &cobra.Command{
	Use:   "indexers",
	Short: "List configured indexers",
	RunE:  runIndexersCmd,
}

func init() {
	rootCmd.AddCommand(indexersCmd)
	indexersCmd.Flags().Bool("test", false, "Test indexer connectivity")
}

func runIndexersCmd(cmd *cobra.Command, args []string) error {
	testFlag, _ := cmd.Flags().GetBool("test")

	client := NewClient(serverURL)
	resp, err := client.Indexers(testFlag)
	if err != nil {
		return fmt.Errorf("failed to fetch indexers: %w", err)
	}

	if jsonOutput {
		printJSON(resp)
		return nil
	}

	if len(resp.Indexers) == 0 {
		fmt.Println("No indexers configured")
		return nil
	}

	fmt.Printf("Indexers (%d):\n\n", len(resp.Indexers))

	if testFlag {
		fmt.Printf("  %-15s %-8s %s\n", "NAME", "STATUS", "LATENCY/ERROR")
		fmt.Println("  " + strings.Repeat("-", 60))
		for _, idx := range resp.Indexers {
			status := idx.Status
			detail := fmt.Sprintf("%dms", idx.ResponseMs)
			if idx.Error != "" {
				detail = idx.Error
			}
			fmt.Printf("  %-15s %-8s %s\n", idx.Name, status, detail)
		}
	} else {
		fmt.Printf("  %-15s %s\n", "NAME", "URL")
		fmt.Println("  " + strings.Repeat("-", 60))
		for _, idx := range resp.Indexers {
			fmt.Printf("  %-15s %s\n", idx.Name, idx.URL)
		}
	}

	return nil
}
