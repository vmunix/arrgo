package tvdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTVDB creates a test server that simulates the TVDB API.
func mockTVDB(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for handler by path
		if handler, ok := handlers[r.URL.Path]; ok {
			handler(w, r)
			return
		}
		// Default: 404
		w.WriteHeader(http.StatusNotFound)
	}))
}

// writeJSON is a test helper that writes JSON response and panics on error.
func writeJSON(w http.ResponseWriter, v any) {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic("test: failed to encode JSON: " + err.Error())
	}
}

// loginHandler returns a handler that validates API key and returns a token.
func loginHandler(validAPIKey, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var body struct {
			APIKey string `json:"apikey"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if body.APIKey != validAPIKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, loginResponse{
			Status: "success",
			Data: struct {
				Token string `json:"token"`
			}{Token: token},
		})
	}
}

// requireAuth wraps a handler with token validation.
func requireAuth(validToken string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+validToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler(w, r)
	}
}

func TestNew(t *testing.T) {
	client := New("test-api-key")
	assert.NotNil(t, client)
	assert.Equal(t, "test-api-key", client.apiKey)
	assert.Equal(t, defaultBaseURL, client.baseURL)
	assert.NotNil(t, client.httpClient)
}

func TestNew_WithOptions(t *testing.T) {
	customHTTP := &http.Client{Timeout: 5 * time.Second}

	client := New("test-key",
		WithBaseURL("https://custom.url"),
		WithHTTPClient(customHTTP),
	)

	assert.Equal(t, "https://custom.url", client.baseURL)
	assert.Same(t, customHTTP, client.httpClient)
}

func TestLogin_Success(t *testing.T) {
	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("valid-key", "jwt-token-123"),
	})
	defer server.Close()

	client := New("valid-key", WithBaseURL(server.URL))
	err := client.login(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "jwt-token-123", client.token)
}

func TestLogin_InvalidAPIKey(t *testing.T) {
	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("valid-key", "jwt-token-123"),
	})
	defer server.Close()

	client := New("wrong-key", WithBaseURL(server.URL))
	err := client.login(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestSearch_Success(t *testing.T) {
	const token = "test-token"

	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("api-key", token),
		"/search": requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Breaking Bad", r.URL.Query().Get("query"))
			assert.Equal(t, "series", r.URL.Query().Get("type"))

			w.Header().Set("Content-Type", "application/json")
			writeJSON(w, searchResponse{
				Status: "success",
				Data: []struct {
					ObjectID string `json:"objectID"`
					Name     string `json:"name"`
					Year     string `json:"year"`
					Status   string `json:"status"`
					Overview string `json:"overview"`
					Network  string `json:"network"`
					TVDBID   string `json:"tvdb_id"`
				}{
					{
						ObjectID: "series-81189",
						Name:     "Breaking Bad",
						Year:     "2008",
						Status:   "Ended",
						Overview: "A high school chemistry teacher...",
						Network:  "AMC",
						TVDBID:   "81189",
					},
					{
						ObjectID: "series-12345",
						Name:     "Breaking Bad: The Movie",
						Year:     "2019",
						Status:   "Ended",
						Overview: "Sequel movie...",
						Network:  "Netflix",
						TVDBID:   "12345",
					},
				},
			})
		}),
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))
	results, err := client.Search(context.Background(), "Breaking Bad")

	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, 81189, results[0].ID)
	assert.Equal(t, "Breaking Bad", results[0].Name)
	assert.Equal(t, 2008, results[0].Year)
	assert.Equal(t, "Ended", results[0].Status)
	assert.Equal(t, "AMC", results[0].Network)

	assert.Equal(t, 12345, results[1].ID)
	assert.Equal(t, "Breaking Bad: The Movie", results[1].Name)
}

func TestSearch_EmptyResults(t *testing.T) {
	const token = "test-token"

	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("api-key", token),
		"/search": requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			writeJSON(w, searchResponse{
				Status: "success",
				Data:   nil,
			})
		}),
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))
	results, err := client.Search(context.Background(), "NonexistentShow12345")

	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestSearch_RateLimited(t *testing.T) {
	const token = "test-token"

	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("api-key", token),
		"/search": requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}),
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))
	_, err := client.Search(context.Background(), "test")

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRateLimited)
}

func TestGetSeries_Success(t *testing.T) {
	const token = "test-token"

	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("api-key", token),
		"/series/81189": requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			writeJSON(w, seriesResponse{
				Status: "success",
				Data: struct {
					ID     int    `json:"id"`
					Name   string `json:"name"`
					Status struct {
						Name string `json:"name"`
					} `json:"status"`
					Overview   string `json:"overview"`
					FirstAired string `json:"firstAired"`
				}{
					ID:   81189,
					Name: "Breaking Bad",
					Status: struct {
						Name string `json:"name"`
					}{Name: "Ended"},
					Overview:   "A high school chemistry teacher diagnosed with terminal lung cancer...",
					FirstAired: "2008-01-20",
				},
			})
		}),
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))
	series, err := client.GetSeries(context.Background(), 81189)

	require.NoError(t, err)
	assert.Equal(t, 81189, series.ID)
	assert.Equal(t, "Breaking Bad", series.Name)
	assert.Equal(t, 2008, series.Year)
	assert.Equal(t, "Ended", series.Status)
	assert.Contains(t, series.Overview, "chemistry teacher")
}

func TestGetSeries_NotFound(t *testing.T) {
	const token = "test-token"

	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("api-key", token),
		"/series/9999999": requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))
	_, err := client.GetSeries(context.Background(), 9999999)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestGetEpisodes_Success(t *testing.T) {
	const token = "test-token"

	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("api-key", token),
		"/series/81189/episodes/default": requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			page := r.URL.Query().Get("page")
			w.Header().Set("Content-Type", "application/json")

			if page == "0" || page == "" {
				writeJSON(w, episodesResponse{
					Status: "success",
					Data: struct {
						Episodes []struct {
							ID           int    `json:"id"`
							SeasonNumber int    `json:"seasonNumber"`
							Number       int    `json:"number"`
							Name         string `json:"name"`
							Overview     string `json:"overview"`
							Aired        string `json:"aired"`
							Runtime      int    `json:"runtime"`
						} `json:"episodes"`
					}{
						Episodes: []struct {
							ID           int    `json:"id"`
							SeasonNumber int    `json:"seasonNumber"`
							Number       int    `json:"number"`
							Name         string `json:"name"`
							Overview     string `json:"overview"`
							Aired        string `json:"aired"`
							Runtime      int    `json:"runtime"`
						}{
							{
								ID:           349232,
								SeasonNumber: 1,
								Number:       1,
								Name:         "Pilot",
								Overview:     "Walter White, a struggling high school chemistry teacher...",
								Aired:        "2008-01-20",
								Runtime:      58,
							},
							{
								ID:           349233,
								SeasonNumber: 1,
								Number:       2,
								Name:         "Cat's in the Bag...",
								Overview:     "Walt and Jesse attempt to tie up loose ends.",
								Aired:        "2008-01-27",
								Runtime:      48,
							},
						},
					},
					Links: struct {
						Next string `json:"next"`
					}{Next: "/series/81189/episodes/default?page=1"},
				})
			} else if page == "1" {
				writeJSON(w, episodesResponse{
					Status: "success",
					Data: struct {
						Episodes []struct {
							ID           int    `json:"id"`
							SeasonNumber int    `json:"seasonNumber"`
							Number       int    `json:"number"`
							Name         string `json:"name"`
							Overview     string `json:"overview"`
							Aired        string `json:"aired"`
							Runtime      int    `json:"runtime"`
						} `json:"episodes"`
					}{
						Episodes: []struct {
							ID           int    `json:"id"`
							SeasonNumber int    `json:"seasonNumber"`
							Number       int    `json:"number"`
							Name         string `json:"name"`
							Overview     string `json:"overview"`
							Aired        string `json:"aired"`
							Runtime      int    `json:"runtime"`
						}{
							{
								ID:           349234,
								SeasonNumber: 1,
								Number:       3,
								Name:         "...And the Bag's in the River",
								Overview:     "Walt wrestles with a difficult decision.",
								Aired:        "2008-02-10",
								Runtime:      48,
							},
						},
					},
					Links: struct {
						Next string `json:"next"`
					}{Next: ""}, // No more pages
				})
			}
		}),
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))
	episodes, err := client.GetEpisodes(context.Background(), 81189)

	require.NoError(t, err)
	require.Len(t, episodes, 3)

	// Check first episode
	assert.Equal(t, 349232, episodes[0].ID)
	assert.Equal(t, 1, episodes[0].Season)
	assert.Equal(t, 1, episodes[0].Episode)
	assert.Equal(t, "Pilot", episodes[0].Name)
	assert.Equal(t, 58, episodes[0].Runtime)
	assert.Equal(t, 2008, episodes[0].AirDate.Year())
	assert.Equal(t, time.January, episodes[0].AirDate.Month())
	assert.Equal(t, 20, episodes[0].AirDate.Day())

	// Check last episode (from second page)
	assert.Equal(t, 349234, episodes[2].ID)
	assert.Equal(t, 1, episodes[2].Season)
	assert.Equal(t, 3, episodes[2].Episode)
}

func TestGetEpisodes_NotFound(t *testing.T) {
	const token = "test-token"

	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("api-key", token),
		"/series/9999999/episodes/default": requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}),
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))
	_, err := client.GetEpisodes(context.Background(), 9999999)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestTokenRefresh_OnExpiry(t *testing.T) {
	var loginCount atomic.Int32
	var requestCount atomic.Int32
	firstToken := "token-1"
	secondToken := "token-2"

	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": func(w http.ResponseWriter, r *http.Request) {
			count := loginCount.Add(1)
			token := firstToken
			if count > 1 {
				token = secondToken
			}

			w.Header().Set("Content-Type", "application/json")
			writeJSON(w, loginResponse{
				Status: "success",
				Data: struct {
					Token string `json:"token"`
				}{Token: token},
			})
		},
		"/series/123": func(w http.ResponseWriter, r *http.Request) {
			count := requestCount.Add(1)
			auth := r.Header.Get("Authorization")

			// First request with first token: return 401 to simulate expiry
			if count == 1 && auth == "Bearer "+firstToken {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Second request with refreshed token: succeed
			if auth == "Bearer "+secondToken {
				w.Header().Set("Content-Type", "application/json")
				writeJSON(w, seriesResponse{
					Status: "success",
					Data: struct {
						ID     int    `json:"id"`
						Name   string `json:"name"`
						Status struct {
							Name string `json:"name"`
						} `json:"status"`
						Overview   string `json:"overview"`
						FirstAired string `json:"firstAired"`
					}{
						ID:         123,
						Name:       "Test Series",
						FirstAired: "2020-01-01",
					},
				})
				return
			}

			w.WriteHeader(http.StatusUnauthorized)
		},
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))
	series, err := client.GetSeries(context.Background(), 123)

	require.NoError(t, err)
	assert.Equal(t, "Test Series", series.Name)
	assert.Equal(t, int32(2), loginCount.Load(), "should have logged in twice")
	assert.Equal(t, int32(2), requestCount.Load(), "should have made two requests")
}

func TestConcurrentRequests_TokenSafety(t *testing.T) {
	const token = "concurrent-token"
	var requestCount atomic.Int32

	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("api-key", token),
		"/search": requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			requestCount.Add(1)
			// Simulate some processing time
			time.Sleep(10 * time.Millisecond)

			w.Header().Set("Content-Type", "application/json")
			writeJSON(w, searchResponse{
				Status: "success",
				Data:   nil,
			})
		}),
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))

	// Make concurrent requests
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := client.Search(context.Background(), "test")
			done <- err
		}()
	}

	// Wait for all requests to complete and collect errors
	var errors []error
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			errors = append(errors, err)
		}
	}
	require.Empty(t, errors, "expected no errors from concurrent requests")

	assert.Equal(t, int32(10), requestCount.Load())
}

func TestContextCancellation(t *testing.T) {
	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": func(w http.ResponseWriter, r *http.Request) {
			// Delay to allow context cancellation
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		},
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.Search(ctx, "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestSearch_ParsesObjectIDFallback(t *testing.T) {
	const token = "test-token"

	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("api-key", token),
		"/search": requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// Some results may not have tvdb_id, only objectID
			writeJSON(w, searchResponse{
				Status: "success",
				Data: []struct {
					ObjectID string `json:"objectID"`
					Name     string `json:"name"`
					Year     string `json:"year"`
					Status   string `json:"status"`
					Overview string `json:"overview"`
					Network  string `json:"network"`
					TVDBID   string `json:"tvdb_id"`
				}{
					{
						ObjectID: "series-99999",
						Name:     "Test Show",
						Year:     "2023",
						TVDBID:   "", // Empty tvdb_id
					},
				},
			})
		}),
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))
	results, err := client.Search(context.Background(), "test")

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 99999, results[0].ID, "should parse ID from objectID")
}

func TestGetSeries_HandlesEmptyFirstAired(t *testing.T) {
	const token = "test-token"

	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("api-key", token),
		"/series/123": requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			writeJSON(w, seriesResponse{
				Status: "success",
				Data: struct {
					ID     int    `json:"id"`
					Name   string `json:"name"`
					Status struct {
						Name string `json:"name"`
					} `json:"status"`
					Overview   string `json:"overview"`
					FirstAired string `json:"firstAired"`
				}{
					ID:         123,
					Name:       "Upcoming Show",
					FirstAired: "", // No first aired date yet
				},
			})
		}),
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))
	series, err := client.GetSeries(context.Background(), 123)

	require.NoError(t, err)
	assert.Equal(t, 0, series.Year, "should be 0 for empty first aired")
}

func TestGetEpisodes_HandlesEmptyAirDate(t *testing.T) {
	const token = "test-token"

	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("api-key", token),
		"/series/123/episodes/default": requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			writeJSON(w, episodesResponse{
				Status: "success",
				Data: struct {
					Episodes []struct {
						ID           int    `json:"id"`
						SeasonNumber int    `json:"seasonNumber"`
						Number       int    `json:"number"`
						Name         string `json:"name"`
						Overview     string `json:"overview"`
						Aired        string `json:"aired"`
						Runtime      int    `json:"runtime"`
					} `json:"episodes"`
				}{
					Episodes: []struct {
						ID           int    `json:"id"`
						SeasonNumber int    `json:"seasonNumber"`
						Number       int    `json:"number"`
						Name         string `json:"name"`
						Overview     string `json:"overview"`
						Aired        string `json:"aired"`
						Runtime      int    `json:"runtime"`
					}{
						{
							ID:           1,
							SeasonNumber: 1,
							Number:       1,
							Name:         "TBA",
							Aired:        "", // No air date yet
						},
					},
				},
			})
		}),
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))
	episodes, err := client.GetEpisodes(context.Background(), 123)

	require.NoError(t, err)
	require.Len(t, episodes, 1)
	assert.True(t, episodes[0].AirDate.IsZero(), "should be zero time for empty air date")
}

func TestServerError(t *testing.T) {
	const token = "test-token"

	server := mockTVDB(t, map[string]http.HandlerFunc{
		"/login": loginHandler("api-key", token),
		"/search": requireAuth(token, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}),
	})
	defer server.Close()

	client := New("api-key", WithBaseURL(server.URL))
	_, err := client.Search(context.Background(), "test")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}
