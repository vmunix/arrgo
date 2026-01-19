# Binary Split Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Split the single `arrgo` binary into `arrgod` (server daemon) and `arrgo` (CLI client).

**Architecture:** Move server code to `cmd/arrgod/`, keep client code in `cmd/arrgo/`. Both import from shared `internal/` and `pkg/` packages. Server starts immediately without subcommands.

**Tech Stack:** Go, Task, air (live reload)

---

### Task 1: Create arrgod main.go

**Files:**
- Create: `cmd/arrgod/main.go`

**Step 1: Create the directory and file**

```go
package main

import (
	"flag"
	"fmt"
	"os"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "config.toml", "Path to config file")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("arrgod %s\n", version)
		os.Exit(0)
	}

	if err := runServer(*configPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
```

**Step 2: Verify it compiles (will fail - runServer not defined yet)**

Run: `go build ./cmd/arrgod`
Expected: FAIL with "undefined: runServer"

**Step 3: Commit skeleton**

```bash
git add cmd/arrgod/main.go
git commit -m "feat(arrgod): add main.go skeleton"
```

---

### Task 2: Move serve.go to arrgod/server.go

**Files:**
- Move: `cmd/arrgo/serve.go` → `cmd/arrgod/server.go`
- Modify: Rename `runServe` to `runServer`

**Step 1: Copy and modify the file**

Copy `cmd/arrgo/serve.go` to `cmd/arrgod/server.go` and change:
- Line 69: `func runServe(configPath string) error {` → `func runServer(configPath string) error {`

All imports and other code stay exactly the same.

**Step 2: Verify arrgod builds**

Run: `go build -o arrgod ./cmd/arrgod`
Expected: SUCCESS

**Step 3: Verify arrgod runs**

Run: `./arrgod --version`
Expected: `arrgod dev`

Run: `./arrgod &` then `curl http://localhost:8484/api/v1/status`
Expected: `{"status":"ok","version":"0.1.0"}`

Kill: `pkill arrgod`

**Step 4: Delete old serve.go**

Run: `rm cmd/arrgo/serve.go`

**Step 5: Commit**

```bash
git add cmd/arrgod/server.go
git rm cmd/arrgo/serve.go
git commit -m "feat(arrgod): move server code from arrgo"
```

---

### Task 3: Update arrgo main.go

**Files:**
- Modify: `cmd/arrgo/main.go`

**Step 1: Remove serve command and update usage**

Replace the entire file with:

```go
package main

import (
	"fmt"
	"os"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}

	switch os.Args[1] {
	case "version", "-v", "--version":
		fmt.Printf("arrgo %s\n", version)
	case "init":
		runInit(os.Args[2:])
	case "status":
		runStatus(os.Args[2:])
	case "search":
		runSearch(os.Args[2:])
	case "queue":
		runQueue(os.Args[2:])
	case "chat":
		fmt.Println("arrgo chat: not yet implemented")
	case "ask":
		fmt.Println("arrgo ask: not yet implemented")
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`arrgo - CLI client for arrgo media automation

Usage:
  arrgo <command> [options]

Setup:
  init               Interactive setup wizard

Commands:
  status             System status (health, disk, queue summary)
  search <query>     Search indexers for content
  queue              Show active downloads

AI Assistant:
  chat               Interactive conversation mode
  ask <question>     One-shot question

Other:
  version            Print version
  help               Show this help

Server:
  Run 'arrgod' to start the server daemon.

Examples:
  arrgo init                    # Set up arrgo
  arrgo search "The Matrix"     # Search for a movie
  arrgo status                  # Check server status`)
}
```

**Step 2: Verify arrgo builds**

Run: `go build -o arrgo ./cmd/arrgo`
Expected: SUCCESS

**Step 3: Verify arrgo runs**

Run: `./arrgo --version`
Expected: `arrgo dev`

Run: `./arrgo help`
Expected: Shows usage without "serve" command

**Step 4: Commit**

```bash
git add cmd/arrgo/main.go
git commit -m "refactor(arrgo): remove serve command, update usage"
```

---

### Task 4: Update Taskfile.yml

**Files:**
- Modify: `Taskfile.yml`

**Step 1: Update for both binaries**

Replace the entire file with:

```yaml
version: '3'

tasks:
  default:
    desc: Show available tasks
    cmds:
      - task --list

  build:
    desc: Build both binaries
    cmds:
      - go build -o arrgo ./cmd/arrgo
      - go build -o arrgod ./cmd/arrgod

  build:client:
    desc: Build CLI client only
    cmds:
      - go build -o arrgo ./cmd/arrgo

  build:server:
    desc: Build server daemon only
    cmds:
      - go build -o arrgod ./cmd/arrgod

  run:
    desc: Run the server
    deps: [build:server]
    cmds:
      - ./arrgod

  test:
    desc: Run all tests
    cmds:
      - go test ./...

  test:v:
    desc: Run tests with verbose output
    cmds:
      - go test -v ./...

  test:cover:
    desc: Run tests with coverage
    cmds:
      - go test -coverprofile=coverage.out ./...
      - go tool cover -html=coverage.out -o coverage.html
      - echo "Coverage report: coverage.html"

  test:integration:
    desc: Run integration tests
    cmds:
      - go test -tags=integration -v ./internal/api/v1/

  lint:
    desc: Run golangci-lint
    cmds:
      - golangci-lint run ./...

  fmt:
    desc: Format code
    cmds:
      - gofmt -w .
      - goimports -w .

  check:
    desc: Run all checks (fmt, lint, test)
    cmds:
      - task: fmt
      - task: lint
      - task: test

  dev:
    desc: Run server with live reload (requires air)
    cmds:
      - air

  generate:
    desc: Run go generate
    cmds:
      - go generate ./...

  clean:
    desc: Clean build artifacts
    cmds:
      - rm -f arrgo arrgod coverage.out coverage.html
      - rm -rf tmp/

  deps:
    desc: Download dependencies
    cmds:
      - go mod download
      - go mod tidy
```

**Step 2: Verify task build works**

Run: `task build`
Expected: Both `arrgo` and `arrgod` binaries created

**Step 3: Commit**

```bash
git add Taskfile.yml
git commit -m "build: update Taskfile for dual binaries"
```

---

### Task 5: Add .air.toml for live reload

**Files:**
- Create: `.air.toml`

**Step 1: Create air config**

```toml
# Air configuration for arrgod live reload
# See: https://github.com/air-verse/air

root = "."
tmp_dir = "tmp"

[build]
cmd = "go build -o ./tmp/arrgod ./cmd/arrgod"
bin = "./tmp/arrgod"
full_bin = "./tmp/arrgod"
include_ext = ["go", "toml"]
exclude_dir = ["tmp", "docs", "cmd/arrgo", ".git"]
exclude_regex = ["_test\\.go$"]
delay = 1000

[log]
time = false

[color]
main = "magenta"
watcher = "cyan"
build = "yellow"
runner = "green"

[misc]
clean_on_exit = true
```

**Step 2: Add tmp/ to .gitignore**

Append to `.gitignore`:
```
tmp/
```

**Step 3: Verify air works**

Run: `air &`
Expected: Server starts, logs show "server starting"

In another terminal: `curl http://localhost:8484/api/v1/status`
Expected: `{"status":"ok","version":"0.1.0"}`

Kill: `pkill -f "tmp/arrgod"`

**Step 4: Commit**

```bash
git add .air.toml .gitignore
git commit -m "build: add air config for server live reload"
```

---

### Task 6: Final verification

**Step 1: Clean and rebuild**

Run: `task clean && task build`
Expected: SUCCESS, both binaries created

**Step 2: Run all tests**

Run: `task test`
Expected: All tests pass

**Step 3: Test server workflow**

```bash
./arrgod &
sleep 2
./arrgo status
./arrgo search --type movie "The Matrix" | head -10
pkill arrgod
```
Expected: All commands work

**Step 4: Test air workflow**

```bash
air &
sleep 3
curl http://localhost:8484/api/v1/status
pkill -f "tmp/arrgod"
```
Expected: Server starts via air

**Step 5: Final commit (if any uncommitted changes)**

```bash
git status
# If clean, skip. Otherwise:
git add -A
git commit -m "chore: binary split complete"
```

---

## Summary

| Task | Description |
|------|-------------|
| 1 | Create `cmd/arrgod/main.go` skeleton |
| 2 | Move `serve.go` → `server.go`, rename function |
| 3 | Update `cmd/arrgo/main.go`, remove serve |
| 4 | Update `Taskfile.yml` for both binaries |
| 5 | Add `.air.toml` for live reload |
| 6 | Final verification |
