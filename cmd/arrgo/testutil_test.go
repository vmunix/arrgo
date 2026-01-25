package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockServer creates an httptest.Server with common test patterns.
// It provides a fluent API for setting up expected request verification
// and response configuration.
type mockServer struct {
	t          *testing.T
	server     *httptest.Server
	handler    http.HandlerFunc
	expectPath string
	expectMeth string
}

// newMockServer creates a new mock server builder.
// Call .Build() to create the actual httptest.Server.
func newMockServer(t *testing.T) *mockServer {
	t.Helper()
	return &mockServer{t: t}
}

// ExpectPath sets the expected request path and verifies it in the handler.
func (m *mockServer) ExpectPath(path string) *mockServer {
	m.expectPath = path
	return m
}

// ExpectMethod sets the expected HTTP method and verifies it in the handler.
func (m *mockServer) ExpectMethod(method string) *mockServer {
	m.expectMeth = method
	return m
}

// ExpectGET is a shorthand for ExpectMethod(http.MethodGet).
func (m *mockServer) ExpectGET() *mockServer {
	return m.ExpectMethod(http.MethodGet)
}

// ExpectPOST is a shorthand for ExpectMethod(http.MethodPost).
func (m *mockServer) ExpectPOST() *mockServer {
	return m.ExpectMethod(http.MethodPost)
}

// ExpectDELETE is a shorthand for ExpectMethod(http.MethodDelete).
func (m *mockServer) ExpectDELETE() *mockServer {
	return m.ExpectMethod(http.MethodDelete)
}

// Handler sets a custom handler function. The function receives the writer
// and request after path/method verification has passed.
func (m *mockServer) Handler(h func(w http.ResponseWriter, r *http.Request)) *mockServer {
	m.handler = h
	return m
}

// RespondJSON sets up a handler that responds with JSON-encoded data.
// The response includes Content-Type: application/json header.
func (m *mockServer) RespondJSON(v any) *mockServer {
	m.handler = func(w http.ResponseWriter, _ *http.Request) {
		respondJSON(m.t, w, v)
	}
	return m
}

// RespondStatus sets up a handler that responds with just a status code.
func (m *mockServer) RespondStatus(code int) *mockServer {
	m.handler = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
	}
	return m
}

// RespondError sets up a handler that responds with an error status and message.
func (m *mockServer) RespondError(code int, message string) *mockServer {
	m.handler = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(message))
	}
	return m
}

// Build creates and returns the httptest.Server.
// The server should be closed with defer srv.Close().
func (m *mockServer) Build() *httptest.Server {
	m.t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify path if expected
		if m.expectPath != "" {
			assert.Equal(m.t, m.expectPath, r.URL.Path, "unexpected request path")
		}
		// Verify method if expected
		if m.expectMeth != "" {
			assert.Equal(m.t, m.expectMeth, r.Method, "unexpected request method")
		}
		// Call the handler if set
		if m.handler != nil {
			m.handler(w, r)
		}
	})

	m.server = httptest.NewServer(handler)
	return m.server
}

// respondJSON writes a JSON response with proper content-type header.
// Fails the test if JSON encoding fails instead of silently ignoring.
func respondJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("failed to encode JSON response: %v", err)
	}
}

// withServerURL temporarily sets serverURL for a test and restores it after.
// Returns a cleanup function that should be deferred.
func withServerURL(url string) func() {
	old := serverURL
	serverURL = url
	return func() { serverURL = old }
}
