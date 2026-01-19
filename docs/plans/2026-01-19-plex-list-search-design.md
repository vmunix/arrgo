# Plex List and Search Commands Design

## Goal

Add commands to query Plex library contents and compare against arrgo's tracking database to identify mismatches (stuck imports, untracked content).

## CLI Interface

```bash
# List commands
arrgo plex list [library]           # list contents, optional library filter
arrgo plex list movies              # case-insensitive library name
arrgo plex list movies --verbose    # detailed output with file paths

# Search commands
arrgo plex search <query>           # search all libraries
arrgo plex search "alien" -l movies # search specific library
```

## Output Formats

**Basic (list default):**
```
Movies (76 items):
  Back to the Future (1985)           ✓ tracked
  Back to the Future Part II (1989)   ✓ tracked
  Alien (1979)                        ✗ not tracked
```

**Detailed (search default, list --verbose):**
```
Back to the Future (1985)
  Path: /data/media/movies/Back to the Future (1985)/...
  Quality: 1080p | Added to Plex: 2026-01-15 | Tracked: yes (id=5)
```

## API Endpoints

- `GET /api/v1/plex/libraries/{name}/items` - list library contents
- `GET /api/v1/plex/search?query=X&library=Y` - search across libraries

## PlexClient Additions

```go
// PlexItem represents a media item in Plex
type PlexItem struct {
    Title     string
    Year      int
    Type      string // movie, show, episode
    AddedAt   int64
    FilePath  string
    Quality   string // parsed from filename/media info
}

// ListLibraryItems returns all items in a library section
func (c *PlexClient) ListLibraryItems(ctx context.Context, sectionKey string) ([]PlexItem, error)

// Search searches for items across libraries
func (c *PlexClient) Search(ctx context.Context, query string) ([]PlexItem, error)
```

## Tracking Comparison

Match Plex items to arrgo content by title + year:
```go
content, _ := store.GetByTitleYear(item.Title, item.Year)
tracked := content != nil
```

## Case-Insensitive Library Names

All plex subcommands (scan, list, search) will match library names case-insensitively:
```go
// User types "movies", matches Plex library "Movies"
strings.EqualFold(userInput, section.Title)
```

## Plex API Endpoints Used

- `GET /library/sections/{key}/all` - list all items in library
- `GET /search?query=X` - search across all libraries
- Item metadata from XML: title, year, addedAt, Media/Part/@file
