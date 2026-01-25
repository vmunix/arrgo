package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List quality profiles",
	RunE:  runProfilesCmd,
}

func init() {
	rootCmd.AddCommand(profilesCmd)
}

func runProfilesCmd(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)
	resp, err := client.Profiles()
	if err != nil {
		return fmt.Errorf("failed to fetch profiles: %w", err)
	}

	if jsonOutput {
		printJSON(resp)
		return nil
	}

	if len(resp.Profiles) == 0 {
		fmt.Println("No quality profiles configured")
		return nil
	}

	fmt.Printf("Quality Profiles (%d):\n\n", len(resp.Profiles))

	fmt.Printf("  %-12s %s\n", "NAME", "RESOLUTIONS")
	fmt.Println("  " + strings.Repeat("-", 50))
	for _, p := range resp.Profiles {
		resolutions := strings.Join(p.Accept, ", ")
		fmt.Printf("  %-12s %s\n", p.Name, resolutions)
	}

	return nil
}
