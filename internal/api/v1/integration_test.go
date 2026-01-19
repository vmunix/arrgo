//go:build integration

package v1

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arrgo/arrgo/internal/download"
	"github.com/arrgo/arrgo/internal/search"
	_ "github.com/mattn/go-sqlite3"
)

// testEnv holds all components needed for integration tests.
type testEnv struct {
	t *testing.T

	// Servers
	api      *httptest.Server // arrgo API under test
	prowlarr *httptest.Server // Mock Prowlarr
	sabnzbd  *httptest.Server // Mock SABnzbd

	// Database
	db *sql.DB

	// Mock response configuration
	prowlarrReleases []search.ProwlarrRelease
	sabnzbdClientID  string
	sabnzbdStatus    *download.ClientStatus
	sabnzbdErr       error
}

func (e *testEnv) cleanup() {
	if e.api != nil {
		e.api.Close()
	}
	if e.prowlarr != nil {
		e.prowlarr.Close()
	}
	if e.sabnzbd != nil {
		e.sabnzbd.Close()
	}
	if e.db != nil {
		_ = e.db.Close()
	}
}
