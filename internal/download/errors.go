package download

import "errors"

// Sentinel errors for the download package.
var (
	// ErrClientUnavailable is returned when the download client cannot be reached.
	ErrClientUnavailable = errors.New("download client unavailable")

	// ErrInvalidAPIKey is returned when the API key is rejected by the client.
	ErrInvalidAPIKey = errors.New("invalid api key")

	// ErrDownloadNotFound is returned when a download is not found in the client.
	ErrDownloadNotFound = errors.New("download not found in client")

	// ErrNotFound is returned when a download record is not found in the database.
	ErrNotFound = errors.New("download not found")
)
