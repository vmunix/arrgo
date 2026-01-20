package importer

import "context"

// MediaServer defines the interface for media server operations.
// This abstraction allows supporting multiple media servers (Plex, Jellyfin, Emby, etc.).
type MediaServer interface {
	// HasContent checks if the media server has content with the given title and year.
	// Used for verification before cleanup.
	HasContent(ctx context.Context, title string, year int) (bool, error)

	// ScanPath triggers a scan of the directory containing the given path.
	// Used to notify the server of newly imported content.
	ScanPath(ctx context.Context, path string) error

	// RefreshLibrary triggers a full refresh of the named library section.
	RefreshLibrary(ctx context.Context, libraryName string) error
}

// Ensure PlexClient implements MediaServer.
var _ MediaServer = (*PlexClient)(nil)
