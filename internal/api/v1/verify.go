package v1

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/vmunix/arrgo/internal/download"
)

// VerifyProblem describes a problem found during verification.
type VerifyProblem struct {
	DownloadID int64    `json:"download_id"`
	Status     string   `json:"status"`
	Title      string   `json:"title"`
	Since      string   `json:"since"`
	Issue      string   `json:"issue"`
	Checks     []string `json:"checks"`
	Likely     string   `json:"likely_cause"`
	Fixes      []string `json:"suggested_fixes"`
}

// VerifyResponse is the response for GET /verify.
type VerifyResponse struct {
	Connections struct {
		Plex    bool   `json:"plex"`
		PlexErr string `json:"plex_error,omitempty"`
		SABnzbd bool   `json:"sabnzbd"`
		SABErr  string `json:"sabnzbd_error,omitempty"`
	} `json:"connections"`
	Checked  int             `json:"checked"`
	Passed   int             `json:"passed"`
	Problems []VerifyProblem `json:"problems"`
}

func (s *Server) verify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check for specific download ID
	idStr := r.URL.Query().Get("id")
	var filterID *int64
	if idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ID", "invalid id")
			return
		}
		filterID = &id
	}

	resp := VerifyResponse{}

	// Test connections
	if s.deps.Plex != nil {
		_, err := s.deps.Plex.GetIdentity(ctx)
		resp.Connections.Plex = err == nil
		if err != nil {
			resp.Connections.PlexErr = err.Error()
		}
	}
	if s.deps.Manager != nil {
		_, err := s.deps.Manager.Client().List(ctx)
		resp.Connections.SABnzbd = err == nil
		if err != nil {
			resp.Connections.SABErr = err.Error()
		}
	}

	// Get downloads to verify (no pagination - verify all active + failed)
	downloads, _, err := s.deps.Downloads.List(download.Filter{Active: true})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", "list downloads: "+err.Error())
		return
	}

	// Also include failed downloads
	failedStatus := download.StatusFailed
	failedDownloads, _, err := s.deps.Downloads.List(download.Filter{Status: &failedStatus})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", "list failed downloads: "+err.Error())
		return
	}
	downloads = append(downloads, failedDownloads...)

	// Filter if specific ID requested
	if filterID != nil {
		filtered := make([]*download.Download, 0)
		for _, dl := range downloads {
			if dl.ID == *filterID {
				filtered = append(filtered, dl)
			}
		}
		downloads = filtered
	}

	resp.Checked = len(downloads)

	for _, dl := range downloads {
		problem := s.verifyDownload(ctx, dl)
		if problem != nil {
			resp.Problems = append(resp.Problems, *problem)
		} else {
			resp.Passed++
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) verifyDownload(ctx context.Context, dl *download.Download) *VerifyProblem {
	content, _ := s.deps.Library.GetContent(dl.ContentID)
	title := dl.ReleaseName
	if content != nil {
		title = content.Title
	}

	since := ""
	if !dl.LastTransitionAt.IsZero() {
		since = time.Since(dl.LastTransitionAt).Round(time.Minute).String()
	}

	switch dl.Status {
	case download.StatusDownloading:
		// Check if actually in SABnzbd
		if s.deps.Manager != nil {
			status, err := s.deps.Manager.Client().Status(ctx, dl.ClientID)
			if err != nil || status == nil {
				return &VerifyProblem{
					DownloadID: dl.ID,
					Status:     string(dl.Status),
					Title:      title,
					Since:      since,
					Issue:      "Not found in SABnzbd queue",
					Checks:     []string{"SABnzbd queue: not found"},
					Likely:     "Download was canceled or SABnzbd cleared it",
					Fixes:      []string{"arrgo retry " + strconv.FormatInt(dl.ID, 10), "arrgo skip " + strconv.FormatInt(dl.ID, 10)},
				}
			}
		}

	case download.StatusCompleted:
		// Check if source file exists
		if s.deps.Manager != nil {
			status, _ := s.deps.Manager.Client().Status(ctx, dl.ClientID)
			if status != nil && status.Path != "" {
				if _, err := os.Stat(status.Path); os.IsNotExist(err) {
					return &VerifyProblem{
						DownloadID: dl.ID,
						Status:     string(dl.Status),
						Title:      title,
						Since:      since,
						Issue:      "Source file not found",
						Checks:     []string{"File at " + status.Path + ": missing"},
						Likely:     "File was manually deleted or moved",
						Fixes:      []string{"arrgo retry " + strconv.FormatInt(dl.ID, 10), "arrgo skip " + strconv.FormatInt(dl.ID, 10)},
					}
				}
			}
		}

	case download.StatusImported:
		// Check if in Plex
		if s.deps.Plex != nil && content != nil {
			found, _ := s.deps.Plex.HasMovie(ctx, content.Title, content.Year)
			if !found {
				return &VerifyProblem{
					DownloadID: dl.ID,
					Status:     string(dl.Status),
					Title:      title,
					Since:      since,
					Issue:      "Not found in Plex library",
					Checks:     []string{"Plex search for '" + content.Title + "': not found"},
					Likely:     "Plex hasn't scanned yet",
					Fixes:      []string{"arrgo plex scan", "Wait for automatic scan"},
				}
			}
		}

	case download.StatusFailed:
		// Failed downloads are always problems
		return &VerifyProblem{
			DownloadID: dl.ID,
			Status:     string(dl.Status),
			Title:      title,
			Since:      since,
			Issue:      "Download failed",
			Checks:     []string{"Status: failed"},
			Likely:     "Download was incomplete, corrupted, or manually failed",
			Fixes:      []string{"arrgo downloads retry " + strconv.FormatInt(dl.ID, 10), "arrgo downloads cancel " + strconv.FormatInt(dl.ID, 10) + " --delete"},
		}
	}

	return nil
}
