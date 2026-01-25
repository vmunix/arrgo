package v1

import (
	"context"
	"errors"

	"github.com/vmunix/arrgo/internal/download"
	"github.com/vmunix/arrgo/internal/events"
	"github.com/vmunix/arrgo/internal/importer"
	"github.com/vmunix/arrgo/internal/library"
	"github.com/vmunix/arrgo/internal/search"
)

// ErrMissingDependency is returned when a required dependency is nil.
var ErrMissingDependency = errors.New("missing required dependency")

// Searcher defines the interface for search functionality.
type Searcher interface {
	Search(ctx context.Context, q search.Query, profile string) (*search.Result, error)
}

// DownloadManager defines the interface for download management.
// Note: Grab is handled via the event bus (GrabRequested event).
type DownloadManager interface {
	Cancel(ctx context.Context, downloadID int64, deleteFiles bool) error
	Client() download.Downloader
	GetActive(ctx context.Context) ([]*download.ActiveDownload, error)
}

// PlexClient defines the interface for Plex media server operations.
type PlexClient interface {
	GetIdentity(ctx context.Context) (*importer.Identity, error)
	GetSections(ctx context.Context) ([]importer.Section, error)
	FindSectionByName(ctx context.Context, name string) (*importer.Section, error)
	GetLibraryCount(ctx context.Context, sectionKey string) (int, error)
	ListLibraryItems(ctx context.Context, sectionKey string) ([]importer.PlexItem, error)
	Search(ctx context.Context, query string) ([]importer.PlexItem, error)
	ScanPath(ctx context.Context, filePath string) error
	RefreshLibrary(ctx context.Context, sectionKey string) error
	HasMovie(ctx context.Context, title string, year int) (bool, error)
	TranslateToLocal(path string) string
}

// FileImporter defines the interface for file import operations.
type FileImporter interface {
	Import(ctx context.Context, downloadID int64, downloadPath string) (*importer.ImportResult, error)
}

// IndexerAPI represents an indexer that can be queried.
type IndexerAPI interface {
	Name() string
	URL() string
	Caps(ctx context.Context) error // Simple connectivity test
}

// ServerDeps contains all dependencies for the API server.
// Required dependencies must be non-nil; optional dependencies may be nil.
type ServerDeps struct {
	// Required dependencies
	Library   *library.Store
	Downloads *download.Store
	History   *importer.HistoryStore

	// Optional dependencies (nil if not configured)
	Searcher Searcher
	Manager  DownloadManager
	Plex     PlexClient
	Importer FileImporter
	Bus      *events.Bus      // Optional: for event-driven mode
	EventLog *events.EventLog // Optional: for event audit log
	Indexers []IndexerAPI     // Optional: configured indexers
}

// Validate checks that all required dependencies are provided.
func (d ServerDeps) Validate() error {
	if d.Library == nil {
		return errors.New("library store is required")
	}
	if d.Downloads == nil {
		return errors.New("downloads store is required")
	}
	if d.History == nil {
		return errors.New("history store is required")
	}
	return nil
}
