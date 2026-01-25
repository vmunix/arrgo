package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientStatus_Success(t *testing.T) {
	srv := newMockServer(t).
		ExpectPath("/api/v1/status").
		ExpectGET().
		RespondJSON(StatusResponse{
			Status:  "ok",
			Version: "1.0.0",
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	status, err := client.Status()
	require.NoError(t, err)
	assert.Equal(t, "ok", status.Status)
	assert.Equal(t, "1.0.0", status.Version)
}

func TestClientStatus_ServerError(t *testing.T) {
	srv := newMockServer(t).
		RespondError(http.StatusInternalServerError, "internal server error").
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Status()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
	assert.Contains(t, err.Error(), "internal server error")
}

func TestClientStatus_ConnectionError(t *testing.T) {
	// Create a server and immediately close it to simulate connection error
	srv := newMockServer(t).Build()
	srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Status()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request failed")
}

func TestClientStatus_InvalidJSON(t *testing.T) {
	srv := newMockServer(t).
		Handler(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("not valid json"))
		}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Status()
	require.Error(t, err)
}

func TestClientStatus_EmptyResponse(t *testing.T) {
	srv := newMockServer(t).
		RespondJSON(StatusResponse{}).
		Build()
	defer srv.Close()

	client := NewClient(srv.URL)
	status, err := client.Status()
	require.NoError(t, err)
	assert.Empty(t, status.Status)
	assert.Empty(t, status.Version)
}
