// Package tmdb provides a client for The Movie Database API.
package tmdb

import "strconv"

// Movie represents TMDB movie metadata.
type Movie struct {
	ID           int64   `json:"id"`
	IMDBID       string  `json:"imdb_id,omitempty"` // e.g., "tt0133093"
	Title        string  `json:"title"`
	Overview     string  `json:"overview"`
	ReleaseDate  string  `json:"release_date"` // "2024-03-01"
	PosterPath   string  `json:"poster_path"`  // "/abc123.jpg"
	BackdropPath string  `json:"backdrop_path"`
	VoteAverage  float64 `json:"vote_average"`
	VoteCount    int     `json:"vote_count"`
	Runtime      int     `json:"runtime"` // minutes
	Genres       []Genre `json:"genres"`
}

// Genre represents a movie genre.
type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Year extracts the year from ReleaseDate.
func (m *Movie) Year() int {
	if len(m.ReleaseDate) < 4 {
		return 0
	}
	year, err := strconv.Atoi(m.ReleaseDate[:4])
	if err != nil {
		return 0
	}
	return year
}

// PosterURL returns the full poster image URL.
// Size can be: w92, w154, w185, w342, w500, w780, original
func (m *Movie) PosterURL(size string) string {
	if m.PosterPath == "" {
		return ""
	}
	return "https://image.tmdb.org/t/p/" + size + m.PosterPath
}
