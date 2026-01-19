# Parser UX Improvements Design

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Date:** 2026-01-19
**Status:** ✅ Complete (2026-01-19)
**Author:** Mark + Claude

## Overview

Surface the improved release parser capabilities to users through three features:

1. **Progressive Quality Profiles** — Richer scoring with HDR, audio, remux preferences
2. **Richer Search Output** — Attribute badges in search results
3. **Debug Command** — `arrgo parse` for inspecting parser output

## 1. Progressive Quality Profiles

### Design Principles

- Omitted field = no preference (accepts any)
- Order within arrays = preference rank (first is best)
- Only specified fields affect scoring
- Users start simple, add granularity as needed

### Configuration Syntax

**Minimal — just resolution:**
```toml
[quality.profiles.simple]
resolution = ["1080p"]
```

**Add layers as needed:**
```toml
[quality.profiles.premium]
resolution = ["2160p", "1080p"]
sources = ["bluray", "webdl"]
codecs = ["x265", "x264"]
hdr = ["dolby-vision", "hdr10+", "hdr10"]
audio = ["atmos", "truehd", "dtshd"]
prefer_remux = false
reject = ["hdtv", "cam"]
```

### Supported Values

| Field | Valid Values |
|-------|--------------|
| resolution | `2160p`, `1080p`, `720p` |
| sources | `bluray`, `webdl`, `webrip`, `hdtv` |
| codecs | `x265`, `x264` |
| hdr | `dolby-vision`, `hdr10+`, `hdr10`, `hdr`, `hlg` |
| audio | `atmos`, `truehd`, `dtshd`, `dts`, `dd+`, `dd`, `aac`, `flac`, `opus` |
| reject | any of the above, plus: `cam`, `ts`, `telesync`, `hdcam` |

### Scoring Algorithm

```
Base score from resolution match:
  - 2160p: 100
  - 1080p: 80
  - 720p: 60
  - unknown: 40

Bonuses (only if field specified in profile):
  - Source match: +10 (adjusted by position: #1=+10, #2=+8, #3=+6, ...)
  - Codec match: +10 (adjusted by position)
  - HDR match: +15 (adjusted by position)
  - Audio match: +15 (adjusted by position)
  - Remux when prefer_remux=true: +20

Penalties:
  - Matches reject list: -10000 (effectively excluded)
```

Position adjustment: `bonus * (1 - 0.2 * position)` where position is 0-indexed.

Example: If `hdr = ["dolby-vision", "hdr10+", "hdr10"]` and release has HDR10:
- HDR10 is position 2
- Bonus = 15 * (1 - 0.2 * 2) = 15 * 0.6 = 9

## 2. Richer Search Output

### Standard Output

```
Found 8 releases for "Dune Part Two":

  # │ RELEASE                                    │     SIZE │ SCORE
────┼────────────────────────────────────────────┼──────────┼──────────────────
  1 │ Dune.Part.Two.2024.2160p.UHD.BluRay...     │  45.2 GB │  142
    │ [2160p] [BluRay] [x265] [DV] [Atmos] [Remux]
  2 │ Dune.Part.Two.2024.2160p.WEB-DL.DDP5.1...  │  12.1 GB │  118
    │ [2160p] [WEB-DL] [x265] [HDR10] [DD+]
  3 │ Dune.Part.Two.2024.1080p.BluRay.x264...    │   8.4 GB │   92
    │ [1080p] [BluRay] [x264]
```

### Badge Display Rules

- Only show badges for detected (non-unknown) attributes
- Badge order: resolution, source, codec, HDR, audio, remux, edition
- Abbreviations: `DV` for Dolby Vision, `DD+` for EAC3/DDP
- Second line only appears if there are badges to show

### Verbose Mode (`--verbose`)

Adds: indexer, release group, edition, streaming service

```
  1 │ Dune.Part.Two.2024.2160p.UHD.BluRay...     │  45.2 GB │  142
    │ [2160p] [BluRay] [x265] [DV] [Atmos] [Remux]
    │ Indexer: NZBgeek  Group: FraMeSToR
```

## 3. Debug Command (`arrgo parse`)

### Basic Usage

```bash
$ arrgo parse "Dune.Part.Two.2024.2160p.UHD.BluRay.REMUX.DV.Atmos-FraMeSToR"

Title:       Dune Part Two
Year:        2024
Resolution:  2160p
Source:      BluRay
Codec:       unknown
HDR:         Dolby Vision
Audio:       Atmos
Remux:       yes
Group:       FraMeSToR
CleanTitle:  dune part two
```

### Score Breakdown (`--score PROFILE`)

```bash
$ arrgo parse "Dune.Part.Two.2024.2160p..." --score premium

Title:       Dune Part Two
Year:        2024
Resolution:  2160p
Source:      BluRay
Codec:       unknown
HDR:         Dolby Vision
Audio:       Atmos
Remux:       yes
Group:       FraMeSToR
CleanTitle:  dune part two

Score Breakdown (profile: premium):
  Resolution (2160p, #1 choice):  +100
  Source (BluRay, #1 choice):      +10
  HDR (DV, #1 choice):             +15
  Audio (Atmos, #1 choice):        +15
  Remux (preferred):               +20
  ─────────────────────────────────────
  Total:                           160
```

### Batch Mode

```bash
$ arrgo parse --file releases.txt --json > parsed.json
```

Reads one release name per line, outputs JSON array.

### Implementation Notes

- Runs entirely client-side using `pkg/release` parser
- No server connection required
- `--score` requires reading config file to load profile definitions

## Documentation Updates

### docs/design.md

1. **Quality Profiles section** — Replace with progressive syntax and examples
2. **CLI Commands section** — Add `arrgo parse` command
3. **Search Module description** — Mention full parsing capabilities

### config.example.toml

Replace quality profiles section:

```toml
# Minimal profile - just resolution
[quality.profiles.sd]
resolution = ["720p", "480p"]

# Standard HD
[quality.profiles.hd]
resolution = ["1080p", "720p"]
sources = ["bluray", "webdl"]

# Premium 4K with HDR/audio preferences
[quality.profiles.uhd]
resolution = ["2160p", "1080p"]
sources = ["bluray", "webdl"]
hdr = ["dolby-vision", "hdr10+", "hdr10"]
audio = ["atmos", "truehd", "dtshd"]
reject = ["hdtv", "cam", "ts"]
```

## Implementation Tasks

### Task 1: Quality Profile Config Parsing

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/quality.go`
- Test: `internal/config/quality_test.go`

Parse the new TOML structure into Go types. Handle missing fields as "no preference".

### Task 2: Scoring Function

**Files:**
- Modify: `internal/search/scorer.go` (or create if doesn't exist)
- Test: `internal/search/scorer_test.go`

Implement the scoring algorithm that uses parsed release info and quality profile.

### Task 3: Search Output Enhancement

**Files:**
- Modify: `cmd/arrgo/commands.go` (printSearchHuman function)

Add second line with attribute badges. Add `--verbose` flag support.

### Task 4: Parse Command

**Files:**
- Create: `cmd/arrgo/parse.go`
- Modify: `cmd/arrgo/main.go` (add command routing)

Implement `arrgo parse` with `--score` and `--file` options.

### Task 5: Documentation

**Files:**
- Modify: `docs/design.md`
- Modify: `config.example.toml`

Update with new quality profile syntax and parse command.

## Out of Scope

- Color coding in terminal output (future enhancement)
- Web UI for profile configuration
- Auto-detection of preferred profile based on content
