package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var eventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Show recent events",
	RunE:  runEventsCmd,
}

func init() {
	rootCmd.AddCommand(eventsCmd)
	eventsCmd.Flags().IntP("limit", "n", 20, "Number of events to show")
}

func runEventsCmd(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")

	client := NewClient(serverURL)
	events, err := client.Events(limit)
	if err != nil {
		return fmt.Errorf("failed to fetch events: %w", err)
	}

	if jsonOutput {
		printJSON(events)
		return nil
	}

	if len(events.Items) == 0 {
		fmt.Println("No events")
		return nil
	}

	fmt.Printf("Recent Events (%d):\n\n", events.Total)
	fmt.Printf("  %-12s %-24s %-15s\n", "TIME", "TYPE", "ENTITY")
	fmt.Println("  " + strings.Repeat("-", 55))

	for _, e := range events.Items {
		t, _ := time.Parse(time.RFC3339, e.OccurredAt)
		ago := formatTimeAgo(t.Unix())
		entity := fmt.Sprintf("%s/%d", e.EntityType, e.EntityID)
		fmt.Printf("  %-12s %-24s %-15s\n", ago, e.EventType, entity)
	}

	return nil
}
