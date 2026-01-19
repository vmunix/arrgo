// internal/importer/plex_test.go
package importer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPlexClient_GetSections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Plex-Token") != "test-token" {
			t.Error("missing or wrong token")
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
  <Directory key="1" title="Movies" type="movie">
    <Location path="/movies"/>
  </Directory>
  <Directory key="2" title="TV Shows" type="show">
    <Location path="/tv"/>
  </Directory>
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token")
	sections, err := client.GetSections(context.Background())
	if err != nil {
		t.Fatalf("GetSections: %v", err)
	}

	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	if sections[0].Key != "1" || sections[0].Title != "Movies" {
		t.Errorf("section 0: got %+v", sections[0])
	}
	if sections[0].Locations[0].Path != "/movies" {
		t.Errorf("section 0 path: got %s", sections[0].Locations[0].Path)
	}
}

func TestPlexClient_ScanPath(t *testing.T) {
	scanCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/library/sections" {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
  <Directory key="1" title="Movies" type="movie">
    <Location path="/movies"/>
  </Directory>
</MediaContainer>`))
			return
		}

		if r.URL.Path == "/library/sections/1/refresh" {
			scanCalled = true
			if r.URL.Query().Get("path") != "/movies/Test Movie (2024)" {
				t.Errorf("wrong path: %s", r.URL.Query().Get("path"))
			}
			w.WriteHeader(http.StatusOK)
			return
		}

		t.Errorf("unexpected path: %s", r.URL.Path)
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token")
	err := client.ScanPath(context.Background(), "/movies/Test Movie (2024)/movie.mkv")
	if err != nil {
		t.Fatalf("ScanPath: %v", err)
	}

	if !scanCalled {
		t.Error("scan endpoint was not called")
	}
}

func TestPlexClient_ScanPath_NoMatchingSection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer>
  <Directory key="1" title="Movies" type="movie">
    <Location path="/movies"/>
  </Directory>
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token")
	err := client.ScanPath(context.Background(), "/other/path/movie.mkv")
	if err == nil {
		t.Error("expected error for non-matching path")
	}
}

func TestPlexClient_ConnectionError(t *testing.T) {
	client := NewPlexClient("http://localhost:99999", "token")
	_, err := client.GetSections(context.Background())
	if err == nil {
		t.Error("expected connection error")
	}
}

func TestPlexClient_GetIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Plex-Token") != "test-token" {
			t.Errorf("missing or wrong token")
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer friendlyName="velcro" version="1.42.2.10156">
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token")
	identity, err := client.GetIdentity(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if identity.Name != "velcro" {
		t.Errorf("name: got %q, want %q", identity.Name, "velcro")
	}
	if identity.Version != "1.42.2.10156" {
		t.Errorf("version: got %q, want %q", identity.Version, "1.42.2.10156")
	}
}

func TestPlexClient_GetIdentity_ConnectionError(t *testing.T) {
	client := NewPlexClient("http://localhost:1", "test-token")
	_, err := client.GetIdentity(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPlexClient_GetSections_WithMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="2">
<Directory key="1" type="movie" title="Movies" scannedAt="1737200000" refreshing="0">
<Location path="/data/media/movies"/>
</Directory>
<Directory key="2" type="show" title="TV Shows" scannedAt="1737100000" refreshing="1">
<Location path="/data/media/tv"/>
</Directory>
</MediaContainer>`))
	}))
	defer server.Close()

	client := NewPlexClient(server.URL, "test-token")
	sections, err := client.GetSections(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sections) != 2 {
		t.Fatalf("sections: got %d, want 2", len(sections))
	}
	if sections[0].ScannedAt != 1737200000 {
		t.Errorf("scannedAt[0]: got %d, want 1737200000", sections[0].ScannedAt)
	}
	if sections[0].Refreshing() != false {
		t.Errorf("refreshing[0]: got %v, want false", sections[0].Refreshing())
	}
	if sections[1].ScannedAt != 1737100000 {
		t.Errorf("scannedAt[1]: got %d, want 1737100000", sections[1].ScannedAt)
	}
	if sections[1].Refreshing() != true {
		t.Errorf("refreshing[1]: got %v, want true", sections[1].Refreshing())
	}
}
