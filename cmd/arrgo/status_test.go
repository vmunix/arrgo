package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientStatus_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(StatusResponse{
			Status:  "ok",
			Version: "1.0.0",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	status, err := client.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("status = %q, want %q", status.Status, "ok")
	}
	if status.Version != "1.0.0" {
		t.Errorf("version = %q, want %q", status.Version, "1.0.0")
	}
}

func TestClientStatus_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Status()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should contain status code 500: %v", err)
	}
	if !strings.Contains(err.Error(), "internal server error") {
		t.Errorf("error should contain response body: %v", err)
	}
}

func TestClientStatus_ConnectionError(t *testing.T) {
	// Create a server and immediately close it to simulate connection error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Status()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error should indicate request failure: %v", err)
	}
}

func TestClientStatus_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	_, err := client.Status()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClientStatus_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(StatusResponse{})
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	status, err := client.Status()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status.Status != "" {
		t.Errorf("status = %q, want empty", status.Status)
	}
	if status.Version != "" {
		t.Errorf("version = %q, want empty", status.Version)
	}
}
