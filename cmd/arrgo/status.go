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
	status, err := client.Status()
	if err != nil {
		return fmt.Errorf("status check failed: %w", err)
	}

	if jsonOutput {
		printJSON(status)
		return nil
	}

	printStatusHumanCobra(serverURL, status)
	return nil
}

func printStatusHumanCobra(server string, s *StatusResponse) {
	fmt.Printf("Server:     %s (%s)\n", server, s.Status)
	fmt.Printf("Version:    %s\n", s.Version)
}
