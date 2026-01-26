// Package tvdb provides a client for the TVDB API v4.
package tvdb

import "time"

// Series represents a TV series from TVDB.
type Series struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Year     int    `json:"year"`      // Extracted from firstAired
	Status   string `json:"status"`    // "Continuing" or "Ended"
	Overview string `json:"overview"`
}

// Episode represents a single episode from TVDB.
type Episode struct {
	ID        int       `json:"id"`
	Season    int       `json:"seasonNumber"`
	Episode   int       `json:"number"`
	Name      string    `json:"name"`
	Overview  string    `json:"overview"`
	AirDate   time.Time `json:"aired"` // Parsed from YYYY-MM-DD
	Runtime   int       `json:"runtime"`
}

// SearchResult represents a series search result.
type SearchResult struct {
	ID       int    `json:"tvdb_id"`
	Name     string `json:"name"`
	Year     int    `json:"year"`
	Status   string `json:"status"`
	Overview string `json:"overview"`
	Network  string `json:"network"`
}

// loginResponse is the TVDB login API response.
type loginResponse struct {
	Status string `json:"status"`
	Data   struct {
		Token string `json:"token"`
	} `json:"data"`
}

// searchResponse is the TVDB search API response.
type searchResponse struct {
	Status string `json:"status"`
	Data   []struct {
		ObjectID string `json:"objectID"`
		Name     string `json:"name"`
		Year     string `json:"year"`
		Status   string `json:"status"`
		Overview string `json:"overview"`
		Network  string `json:"network"`
		TVDBID   string `json:"tvdb_id"`
	} `json:"data"`
}

// seriesResponse is the TVDB get series API response.
type seriesResponse struct {
	Status string `json:"status"`
	Data   struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Status struct {
			Name string `json:"name"`
		} `json:"status"`
		Overview   string `json:"overview"`
		FirstAired string `json:"firstAired"` // YYYY-MM-DD
	} `json:"data"`
}

// episodesResponse is the TVDB get episodes API response.
type episodesResponse struct {
	Status string `json:"status"`
	Data   struct {
		Episodes []struct {
			ID           int    `json:"id"`
			SeasonNumber int    `json:"seasonNumber"`
			Number       int    `json:"number"`
			Name         string `json:"name"`
			Overview     string `json:"overview"`
			Aired        string `json:"aired"` // YYYY-MM-DD
			Runtime      int    `json:"runtime"`
		} `json:"episodes"`
	} `json:"data"`
	Links struct {
		Next string `json:"next"`
	} `json:"links"`
}
