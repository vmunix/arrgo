// internal/api/v1/types.go
package v1

import "time"

// contentResponse is the API representation of content.
type contentResponse struct {
	ID             int64     `json:"id"`
	Type           string    `json:"type"`
	TMDBID         *int64    `json:"tmdb_id,omitempty"`
	TVDBID         *int64    `json:"tvdb_id,omitempty"`
	Title          string    `json:"title"`
	Year           int       `json:"year"`
	Status         string    `json:"status"`
	QualityProfile string    `json:"quality_profile"`
	RootPath       string    `json:"root_path"`
	AddedAt        time.Time `json:"added_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// listContentResponse is the response for GET /content.
type listContentResponse struct {
	Items  []contentResponse `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

// addContentRequest is the request body for POST /content.
type addContentRequest struct {
	Type           string `json:"type"`
	TMDBID         *int64 `json:"tmdb_id,omitempty"`
	TVDBID         *int64 `json:"tvdb_id,omitempty"`
	Title          string `json:"title"`
	Year           int    `json:"year"`
	QualityProfile string `json:"quality_profile"`
	RootPath       string `json:"root_path,omitempty"`
}

// updateContentRequest is the request body for PUT /content/:id.
type updateContentRequest struct {
	Status         *string `json:"status,omitempty"`
	QualityProfile *string `json:"quality_profile,omitempty"`
}
