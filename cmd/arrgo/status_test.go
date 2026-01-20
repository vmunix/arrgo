package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientStatus_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/status", r.URL.Path, "unexpected path")
		assert.Equal(t, http.MethodGet, r.Method, "unexpected method")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(StatusResponse{
			Status:  "ok",
			Version: "1.0.0",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	status, err := client.Status()
	require.NoError(t, err)
	assert.Equal(t, "ok", status.Status)
	assert.Equal(t, "1.0.0", status.Version)
}

func TestClientStatus_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Status()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "internal server error")
}

func TestClientStatus_ConnectionError(t *testing.T) {
	// Create a server and immediately close it to simulate connection error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Status()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request failed")
}

func TestClientStatus_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Status()
	require.Error(t, err)
}

func TestClientStatus_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(StatusResponse{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	status, err := client.Status()
	require.NoError(t, err)
	assert.Empty(t, status.Status)
	assert.Empty(t, status.Version)
}
