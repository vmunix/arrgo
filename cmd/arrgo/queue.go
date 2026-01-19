package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Show active downloads",
	RunE:  runQueueCmd,
}

func init() {
	rootCmd.AddCommand(queueCmd)
	queueCmd.Flags().BoolP("all", "a", false, "Include completed/imported downloads")
}

func runQueueCmd(cmd *cobra.Command, args []string) error {
	showAll, _ := cmd.Flags().GetBool("all")

	client := NewClient(serverURL)
	downloads, err := client.Downloads(!showAll)
	if err != nil {
		return fmt.Errorf("queue fetch failed: %w", err)
	}

	if jsonOutput {
		printJSON(downloads)
		return nil
	}

	printQueueHumanCobra(downloads)
	return nil
}

func printQueueHumanCobra(d *ListDownloadsResponse) {
	if len(d.Items) == 0 {
		fmt.Println("No downloads in queue")
		return
	}

	fmt.Printf("Downloads (%d):\n\n", d.Total)
	fmt.Printf("  # │ %-40s │ %-12s │ %s\n", "TITLE", "STATUS", "INDEXER")
	fmt.Println("────┼──────────────────────────────────────────┼──────────────┼─────────")

	for i, dl := range d.Items {
		title := dl.ReleaseName
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		fmt.Printf(" %2d │ %-40s │ %-12s │ %s\n", i+1, title, dl.Status, dl.Indexer)
	}
}
