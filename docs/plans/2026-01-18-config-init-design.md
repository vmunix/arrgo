# Config Init Command Design

**Date:** 2026-01-18
**Status:** Approved

## Overview

Interactive setup wizard that creates `config.toml` with essential values. Prompts for indexer, downloader, and media paths - everything needed to start using arrgo.

## Design Decisions

1. **Prompt for essentials only** - Prowlarr, SABnzbd, media paths. Other sections use defaults.
2. **Write to current directory** - Creates `config.toml`, errors if exists (use `--force`)
3. **Simple stdin prompts** - No dependencies, defaults shown in brackets
4. **Visible API key input** - No terminal raw mode complexity for a one-time wizard

## Command Behavior

```
$ arrgo init

arrgo setup wizard

Prowlarr URL [http://localhost:9696]:
Prowlarr API Key: ****

SABnzbd URL [http://localhost:8085]:
SABnzbd API Key: ****
SABnzbd Category [arrgo]:

Movies path [/movies]: /srv/media/movies
Series path [/tv]: /srv/media/tv

Config written to config.toml
Run 'arrgo serve' to start the server.
```

## Prompted Values

| Value | Default | Required |
|-------|---------|----------|
| Prowlarr URL | `http://localhost:9696` | No (has default) |
| Prowlarr API Key | - | Yes |
| SABnzbd URL | `http://localhost:8085` | No (has default) |
| SABnzbd API Key | - | Yes |
| SABnzbd Category | `arrgo` | No (has default) |
| Movies path | `/movies` | No (has default) |
| Series path | `/tv` | No (has default) |

## Implementation

**Files:**
- Modify: `cmd/arrgo/main.go` - Wire up init command
- Create: `cmd/arrgo/init.go` - Init command logic

**Functions:**
- `runInit(args []string)` - Entry point, parses --force flag
- `prompt(label, defaultVal string) string` - Shows prompt, returns input or default
- `writeConfig(values) error` - Generates TOML from template, writes file

**Config template:** Embedded string constant with prompted values interpolated. All other sections use sensible defaults matching `config.example.toml`.

## Error Handling

- **File exists**: `"config.toml already exists. Use --force to overwrite."`
- **Empty required value**: Re-prompt with `"Value required"`
- **Write failure**: Exit with error message
- **Ctrl+C**: Exit cleanly, no partial file

## Future (v2)

- Connection validation before writing (test Prowlarr/SABnzbd reachability)
