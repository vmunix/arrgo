// Package search handles indexer queries and release matching.
package search

import "errors"

var (
	// ErrProwlarrUnavailable indicates Prowlarr could not be reached.
	ErrProwlarrUnavailable = errors.New("prowlarr unavailable")

	// ErrInvalidAPIKey indicates the Prowlarr API key is invalid.
	ErrInvalidAPIKey = errors.New("invalid prowlarr api key")

	// ErrNoResults indicates no matching releases were found.
	// This is informational, not a failure.
	ErrNoResults = errors.New("no matching releases found")
)
