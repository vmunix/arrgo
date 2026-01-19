# Config Init Implementation Plan

**Status:** ✅ Complete

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Interactive setup wizard that creates config.toml with essential values.

**Architecture:** Single file `cmd/arrgo/init.go` with prompt loop and TOML template. Reuse existing `prompt()` helper from commands.go.

**Tech Stack:** Go stdlib (bufio, os, flag, strings, text/template)

---

### Task 1: Create init.go with runInit entry point

**Files:**
- Create: `cmd/arrgo/init.go`
- Modify: `cmd/arrgo/main.go:28-29`

**Step 1: Write the test**

No unit test needed - this is a simple CLI wiring task. Manual verification.

**Step 2: Create init.go with flag parsing**

```go
package main

import (
	"flag"
	"fmt"
	"os"
)

func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	force := fs.Bool("force", false, "Overwrite existing config.toml")
	_ = fs.Parse(args)

	configPath := "config.toml"

	// Check if config exists
	if _, err := os.Stat(configPath); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "config.toml already exists. Use --force to overwrite.\n")
		os.Exit(1)
	}

	fmt.Println("arrgo setup wizard")
	fmt.Println()

	// TODO: Prompt for values and write config
	fmt.Println("Not yet implemented")
}
```

**Step 3: Wire up in main.go**

Replace the placeholder:
```go
case "init":
	runInit(os.Args[2:])
```

**Step 4: Verify**

Run: `go build -o arrgo ./cmd/arrgo && ./arrgo init`
Expected: Shows "arrgo setup wizard" message

Run: `touch config.toml && ./arrgo init`
Expected: "config.toml already exists. Use --force to overwrite."

Run: `./arrgo init --force`
Expected: Shows wizard (ignores existing file)

**Step 5: Commit**

```bash
git add cmd/arrgo/init.go cmd/arrgo/main.go
git commit -m "feat(cli): add init command skeleton with --force flag"
```

---

### Task 2: Add promptWithDefault helper

**Files:**
- Modify: `cmd/arrgo/init.go`

**Step 1: Add promptWithDefault function**

The existing `prompt()` in commands.go doesn't show defaults. Add a new helper:

```go
// promptWithDefault shows a prompt with default value in brackets.
// Returns the user's input, or the default if input is empty.
func promptWithDefault(label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	return input
}
```

Add imports: `"bufio"`, `"strings"`

**Step 2: Verify**

This will be tested as part of Task 3.

**Step 3: Commit**

```bash
git add cmd/arrgo/init.go
git commit -m "feat(cli): add promptWithDefault helper for init wizard"
```

---

### Task 3: Add promptRequired helper

**Files:**
- Modify: `cmd/arrgo/init.go`

**Step 1: Add promptRequired function**

For API keys that have no default:

```go
// promptRequired prompts until a non-empty value is provided.
func promptRequired(label string) string {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s: ", label)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			return input
		}
		fmt.Println("  Value required")
	}
}
```

**Step 2: Commit**

```bash
git add cmd/arrgo/init.go
git commit -m "feat(cli): add promptRequired helper for required values"
```

---

### Task 4: Implement the prompt flow

**Files:**
- Modify: `cmd/arrgo/init.go`

**Step 1: Add config values struct and prompts**

```go
type initConfig struct {
	ProwlarrURL    string
	ProwlarrAPIKey string
	SABnzbdURL     string
	SABnzbdAPIKey  string
	SABnzbdCategory string
	MoviesPath     string
	SeriesPath     string
}

func gatherConfig() initConfig {
	var cfg initConfig

	cfg.ProwlarrURL = promptWithDefault("Prowlarr URL", "http://localhost:9696")
	cfg.ProwlarrAPIKey = promptRequired("Prowlarr API Key")
	fmt.Println()

	cfg.SABnzbdURL = promptWithDefault("SABnzbd URL", "http://localhost:8085")
	cfg.SABnzbdAPIKey = promptRequired("SABnzbd API Key")
	cfg.SABnzbdCategory = promptWithDefault("SABnzbd Category", "arrgo")
	fmt.Println()

	cfg.MoviesPath = promptWithDefault("Movies path", "/movies")
	cfg.SeriesPath = promptWithDefault("Series path", "/tv")

	return cfg
}
```

**Step 2: Update runInit to call gatherConfig**

```go
func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	force := fs.Bool("force", false, "Overwrite existing config.toml")
	_ = fs.Parse(args)

	configPath := "config.toml"

	// Check if config exists
	if _, err := os.Stat(configPath); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "config.toml already exists. Use --force to overwrite.\n")
		os.Exit(1)
	}

	fmt.Println("arrgo setup wizard")
	fmt.Println()

	cfg := gatherConfig()

	// TODO: Write config file
	_ = cfg
}
```

**Step 3: Verify**

Run: `rm -f config.toml && go build -o arrgo ./cmd/arrgo && ./arrgo init`
Expected: Prompts appear with defaults, required fields re-prompt if empty

**Step 4: Commit**

```bash
git add cmd/arrgo/init.go
git commit -m "feat(cli): implement init wizard prompt flow"
```

---

### Task 5: Add config template and write function

**Files:**
- Modify: `cmd/arrgo/init.go`

**Step 1: Add config template constant**

```go
const configTemplate = `# arrgo configuration
# Generated by arrgo init

[server]
host = "0.0.0.0"
port = 8484
log_level = "info"

[database]
path = "./data/arrgo.db"

[libraries.movies]
root = "{{.MoviesPath}}"
naming = "{title} ({year})/{title} ({year}) [{quality}].{ext}"

[libraries.series]
root = "{{.SeriesPath}}"
naming = "{title}/Season {season:02d}/{title} - S{season:02d}E{episode:02d} [{quality}].{ext}"

[quality]
default = "hd"

[quality.profiles.hd]
accept = ["1080p bluray", "1080p webdl", "1080p hdtv", "720p bluray", "720p webdl"]

[quality.profiles.uhd]
accept = ["2160p bluray", "2160p webdl", "1080p bluray", "1080p webdl"]

[quality.profiles.any]
accept = ["2160p", "1080p", "720p", "480p"]

[indexers.prowlarr]
url = "{{.ProwlarrURL}}"
api_key = "{{.ProwlarrAPIKey}}"

[downloaders.sabnzbd]
url = "{{.SABnzbdURL}}"
api_key = "{{.SABnzbdAPIKey}}"
category = "{{.SABnzbdCategory}}"

[notifications.plex]
url = "http://localhost:32400"
token = ""
libraries = ["Movies", "TV Shows"]

[ai]
enabled = false
provider = "ollama"

[ai.ollama]
url = "http://localhost:11434"
model = "llama3.1:8b"
`
```

**Step 2: Add writeConfig function**

```go
func writeConfig(cfg initConfig, path string) error {
	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, cfg); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
```

Add import: `"text/template"`

**Step 3: Update runInit to write config**

```go
func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	force := fs.Bool("force", false, "Overwrite existing config.toml")
	_ = fs.Parse(args)

	configPath := "config.toml"

	// Check if config exists
	if _, err := os.Stat(configPath); err == nil && !*force {
		fmt.Fprintf(os.Stderr, "config.toml already exists. Use --force to overwrite.\n")
		os.Exit(1)
	}

	fmt.Println("arrgo setup wizard")
	fmt.Println()

	cfg := gatherConfig()

	if err := writeConfig(cfg, configPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Config written to config.toml")
	fmt.Println("Run 'arrgo serve' to start the server.")
}
```

**Step 4: Verify**

Run: `rm -f config.toml && go build -o arrgo ./cmd/arrgo && echo -e "http://localhost:9696\ntest-api-key\nhttp://localhost:8085\nsab-api-key\narrgo\n/movies\n/tv" | ./arrgo init`
Expected: config.toml created with entered values

Run: `cat config.toml`
Expected: Valid TOML with interpolated values

**Step 5: Commit**

```bash
git add cmd/arrgo/init.go
git commit -m "feat(cli): complete init wizard with config generation"
```

---

### Task 6: Final verification

**Step 1: Build and test full flow**

```bash
go build -o arrgo ./cmd/arrgo
rm -f config.toml
./arrgo init
# Enter test values interactively
cat config.toml
```

**Step 2: Run lint**

```bash
golangci-lint run ./cmd/arrgo/...
```

Fix any issues.

**Step 3: Verify config loads**

The generated config should be loadable by `arrgo serve` (though services won't be reachable with fake keys).

**Step 4: Final commit if needed**

If lint fixes required:
```bash
git add -A
git commit -m "fix(cli): address lint issues in init command"
```
