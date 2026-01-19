# Cobra CLI Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate arrgo CLI from manual flag parsing to Cobra framework for robust flag handling.

**Architecture:** File-per-command structure with persistent global flags on root. Each command file registers itself via init().

**Tech Stack:** github.com/spf13/cobra

---

### Task 1: Add Cobra Dependency

**Files:**
- Modify: `go.mod`

**Step 1: Add Cobra dependency**

Run: `go get github.com/spf13/cobra@latest`

**Step 2: Verify dependency added**

Run: `grep cobra go.mod`
Expected: `github.com/spf13/cobra v1.x.x`

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add cobra dependency"
```

---

### Task 2: Create Root Command

**Files:**
- Create: `cmd/arrgo/root.go`
- Modify: `cmd/arrgo/main.go`

**Step 1: Create root.go with persistent flags**

```go
package main

import (
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var (
	serverURL  string
	jsonOutput bool
)

var rootCmd = &cobra.Command{
	Use:   "arrgo",
	Short: "CLI client for arrgo media automation",
	Long: `arrgo - CLI client for arrgo media automation

A unified CLI for searching indexers, managing downloads,
and automating your media library.

Run 'arrgod' to start the server daemon.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&serverURL, "server", "http://localhost:8484", "Server URL")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	rootCmd.Version = version
	rootCmd.SetVersionTemplate("arrgo {{.Version}}\n")
}
```

**Step 2: Replace main.go**

```go
package main

func main() {
	Execute()
}
```

**Step 3: Verify it compiles**

Run: `go build ./cmd/arrgo`
Expected: No errors

**Step 4: Verify help works**

Run: `./arrgo --help`
Expected: Shows "arrgo - CLI client for arrgo media automation"

**Step 5: Verify version works**

Run: `./arrgo --version`
Expected: "arrgo dev"

**Step 6: Commit**

```bash
git add cmd/arrgo/root.go cmd/arrgo/main.go
git commit -m "feat(cli): add cobra root command with persistent flags"
```

---

### Task 3: Convert Status Command

**Files:**
- Create: `cmd/arrgo/status.go`

**Step 1: Create status.go**

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "System status (health, disk, queue summary)",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	client := NewClient(serverURL)
	status, err := client.Status()
	if err != nil {
		return fmt.Errorf("status check failed: %w", err)
	}

	if jsonOutput {
		printJSON(status)
		return nil
	}

	printStatusHuman(serverURL, status)
	return nil
}

func printStatusHuman(server string, s *StatusResponse) {
	fmt.Printf("Server:     %s (%s)\n", server, s.Status)
	fmt.Printf("Version:    %s\n", s.Version)
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`
Expected: No errors

**Step 3: Verify command shows in help**

Run: `./arrgo --help`
Expected: Shows "status" under "Available Commands"

**Step 4: Commit**

```bash
git add cmd/arrgo/status.go
git commit -m "feat(cli): convert status command to cobra"
```

---

### Task 4: Convert Queue Command

**Files:**
- Create: `cmd/arrgo/queue.go`

**Step 1: Create queue.go**

```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var queueCmd = &cobra.Command{
	Use:   "queue",
	Short: "Show active downloads",
	RunE:  runQueue,
}

func init() {
	rootCmd.AddCommand(queueCmd)
	queueCmd.Flags().BoolP("all", "a", false, "Include completed/imported downloads")
}

func runQueue(cmd *cobra.Command, args []string) error {
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

	printQueueHuman(downloads)
	return nil
}

func printQueueHuman(d *ListDownloadsResponse) {
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
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`
Expected: No errors

**Step 3: Commit**

```bash
git add cmd/arrgo/queue.go
git commit -m "feat(cli): convert queue command to cobra"
```

---

### Task 5: Convert Search Command

**Files:**
- Create: `cmd/arrgo/search.go`

**Step 1: Create search.go**

```go
package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/arrgo/arrgo/pkg/release"
	"github.com/spf13/cobra"
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
	RunE: runSearch,
}

func init() {
	rootCmd.AddCommand(searchCmd)
	searchCmd.Flags().BoolP("verbose", "v", false, "Show indexer, group, service")
	searchCmd.Flags().String("type", "", "Content type (movie or series)")
	searchCmd.Flags().String("profile", "", "Quality profile")
	searchCmd.Flags().String("grab", "", "Grab release: number or 'best'")
}

func runSearch(cmd *cobra.Command, args []string) error {
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

	printSearchHuman(query, results, verbose)

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

func printSearchHuman(query string, r *SearchResponse, verbose bool) {
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
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`
Expected: No errors

**Step 3: Commit**

```bash
git add cmd/arrgo/search.go
git commit -m "feat(cli): convert search command to cobra"
```

---

### Task 6: Convert Init Command

**Files:**
- Modify: `cmd/arrgo/init.go`

**Step 1: Update init.go to Cobra pattern**

Replace the runInit function and add Cobra command. Keep all the existing helper functions (gatherConfig, writeConfig, promptWithDefault, promptRequired, configTemplate, initConfig struct).

Add at the top of the file after the imports:

```go
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive setup wizard",
	Long:  `Interactive setup wizard to create config.toml.`,
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().Bool("force", false, "Overwrite existing config.toml")
}
```

Replace the runInit function:

```go
func runInit(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	configPath := "config.toml"

	// Check if config exists
	if _, err := os.Stat(configPath); err == nil && !force {
		return fmt.Errorf("config.toml already exists. Use --force to overwrite")
	}

	fmt.Println("arrgo setup wizard")
	fmt.Println()

	cfg := gatherConfig()

	if err := writeConfig(cfg, configPath); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Println()
	fmt.Println("Config written to config.toml")
	fmt.Println("Run 'arrgod' to start the server.")
	return nil
}
```

Remove the old flag import and flag.NewFlagSet code.

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`
Expected: No errors

**Step 3: Commit**

```bash
git add cmd/arrgo/init.go
git commit -m "feat(cli): convert init command to cobra"
```

---

### Task 7: Convert Parse Command

**Files:**
- Modify: `cmd/arrgo/parse.go`

**Step 1: Update parse.go to Cobra pattern**

Add Cobra command definition near the top after type definitions:

```go
var parseCmd = &cobra.Command{
	Use:   "parse [flags] <release-name>",
	Short: "Parse release name (local, no server needed)",
	Long: `Parse a release name to extract metadata.

Examples:
  arrgo parse "The.Matrix.1999.2160p.UHD.BluRay.x265-GROUP"
  arrgo parse --score hd "Movie.2024.1080p.WEB-DL.x264-GROUP"
  arrgo parse --file releases.txt --json`,
	RunE: runParse,
}

func init() {
	rootCmd.AddCommand(parseCmd)
	parseCmd.Flags().String("score", "", "Score against quality profile")
	parseCmd.Flags().StringP("file", "f", "", "Read release names from file (one per line)")
	parseCmd.Flags().String("config", "config.toml", "Path to config file")
	// Note: --json is inherited from root as persistent flag
}
```

Replace the runParse function signature and update flag access:

```go
func runParse(cmd *cobra.Command, args []string) error {
	scoreProfile, _ := cmd.Flags().GetString("score")
	inputFile, _ := cmd.Flags().GetString("file")
	configPath, _ := cmd.Flags().GetString("config")

	// Determine input mode
	var releaseNames []string
	if inputFile != "" {
		// Batch mode: read from file
		names, err := readReleaseFile(inputFile)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}
		releaseNames = names
	} else if len(args) > 0 {
		// Single release from command line
		releaseNames = []string{args[0]}
	} else {
		return fmt.Errorf("usage: arrgo parse <release-name> or arrgo parse --file <filename>")
	}

	// Load config if scoring is requested
	var cfg *config.Config
	if scoreProfile != "" {
		var err error
		cfg, err = config.LoadWithoutValidation(configPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		// Verify profile exists
		if _, ok := cfg.Quality.Profiles[scoreProfile]; !ok {
			return fmt.Errorf("profile '%s' not found. Available: %s",
				scoreProfile, strings.Join(getProfileNames(cfg), ", "))
		}
	}

	// Parse all releases
	results := make([]ParseResult, 0, len(releaseNames))
	for _, name := range releaseNames {
		info := release.Parse(name)
		result := ParseResult{Info: info}

		if scoreProfile != "" && cfg != nil {
			profile := cfg.Quality.Profiles[scoreProfile]
			result.Profile = scoreProfile
			result.Score, result.Breakdown = scoreWithBreakdown(*info, profile)
		}

		results = append(results, result)
	}

	// Output results (use global jsonOutput)
	if jsonOutput {
		outputJSON(results)
	} else {
		for i, result := range results {
			if i > 0 {
				fmt.Println()
			}
			printHumanReadable(result)
		}
	}
	return nil
}
```

Remove the old flag import usage.

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`
Expected: No errors

**Step 3: Commit**

```bash
git add cmd/arrgo/parse.go
git commit -m "feat(cli): convert parse command to cobra"
```

---

### Task 8: Add Completion Command

**Files:**
- Create: `cmd/arrgo/completion.go`

**Step 1: Create completion.go**

```go
package main

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for arrgo.

To load completions:

Bash:
  $ source <(arrgo completion bash)
  # To load completions for each session, execute once:
  # Linux:
  $ arrgo completion bash > /etc/bash_completion.d/arrgo
  # macOS:
  $ arrgo completion bash > $(brew --prefix)/etc/bash_completion.d/arrgo

Zsh:
  $ source <(arrgo completion zsh)
  # To load completions for each session, execute once:
  $ arrgo completion zsh > "${fpath[1]}/_arrgo"

Fish:
  $ arrgo completion fish | source
  # To load completions for each session, execute once:
  $ arrgo completion fish > ~/.config/fish/completions/arrgo.fish

PowerShell:
  PS> arrgo completion powershell | Out-String | Invoke-Expression
  # To load completions for each session, execute once:
  PS> arrgo completion powershell > arrgo.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
```

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`
Expected: No errors

**Step 3: Verify completion generates**

Run: `./arrgo completion bash | head -5`
Expected: Bash completion script header

**Step 4: Commit**

```bash
git add cmd/arrgo/completion.go
git commit -m "feat(cli): add shell completion command"
```

---

### Task 9: Clean Up Old Code

**Files:**
- Modify: `cmd/arrgo/commands.go`

**Step 1: Remove migrated code from commands.go**

Keep only:
- `printJSON` function
- `formatSize` function
- `prompt` function
- `buildBadges` function
- `grabRelease` function

Remove:
- `commonFlags` struct
- `parseCommonFlags` function
- `runStatus` function (moved to status.go)
- `printStatusHuman` function (moved to status.go)
- `runQueue` function (moved to queue.go)
- `printQueueHuman` function (moved to queue.go)
- `runSearch` function (moved to search.go)
- `printSearchHuman` function (moved to search.go)

Remove the `flag` import if no longer needed.

**Step 2: Verify it compiles**

Run: `go build ./cmd/arrgo`
Expected: No errors

**Step 3: Verify all commands work**

Run:
```bash
./arrgo --help
./arrgo status --help
./arrgo search --help
./arrgo parse --help
./arrgo queue --help
./arrgo init --help
./arrgo completion --help
```
Expected: All show appropriate help text

**Step 4: Commit**

```bash
git add cmd/arrgo/commands.go
git commit -m "refactor(cli): remove migrated code from commands.go"
```

---

### Task 10: Test Flag Ordering

**Files:** None (manual testing)

**Step 1: Test flags after positional args**

Run: `./arrgo parse "The.Matrix.1999.1080p.BluRay.x264-GROUP" --json`
Expected: JSON output (not error about flags)

**Step 2: Test search with flags after query**

Start server: `./arrgod &`

Run: `./arrgo search "matrix" --verbose 2>&1 | head -20`
Expected: Shows verbose output with Indexer/Group info

Kill server: `pkill arrgod`

**Step 3: Test persistent flags work on subcommands**

Run: `./arrgo --json parse "The.Matrix.1999.1080p.BluRay.x264-GROUP"`
Expected: JSON output

**Step 4: Commit test verification**

```bash
git commit --allow-empty -m "test: verify cobra flag ordering works correctly"
```

---

### Task 11: Run Full Test Suite

**Files:** None

**Step 1: Run all tests**

Run: `go test ./...`
Expected: All tests pass

**Step 2: Run linter**

Run: `golangci-lint run ./...`
Expected: No errors (warnings OK)

**Step 3: Build both binaries**

Run: `go build ./cmd/...`
Expected: Both arrgo and arrgod build successfully

**Step 4: Final commit**

```bash
git add -A
git commit -m "chore: cobra migration complete" --allow-empty
```
