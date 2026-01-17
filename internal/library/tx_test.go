// internal/library/tx_test.go
package library

import (
	"errors"
	"testing"
)

func TestTx_Commit(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	c := &Content{Type: ContentTypeMovie, Title: "TX Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := tx.AddContent(c); err != nil {
		t.Fatalf("AddContent in tx failed: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Should be visible outside transaction
	got, err := store.GetContent(c.ID)
	if err != nil {
		t.Fatalf("GetContent after commit failed: %v", err)
	}
	if got.Title != "TX Movie" {
		t.Errorf("expected title 'TX Movie', got %q", got.Title)
	}
}

func TestTx_Rollback_Comprehensive(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	c := &Content{Type: ContentTypeMovie, Title: "TX Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := tx.AddContent(c); err != nil {
		t.Fatalf("AddContent in tx failed: %v", err)
	}
	id := c.ID

	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Should NOT be visible outside transaction
	_, err = store.GetContent(id)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after rollback, got %v", err)
	}
}

func TestTx_MultipleOperations(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Add series with episodes in one transaction
	series := &Content{Type: ContentTypeSeries, Title: "TX Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := tx.AddContent(series); err != nil {
		t.Fatalf("AddContent failed: %v", err)
	}

	for i := 1; i <= 3; i++ {
		ep := &Episode{ContentID: series.ID, Season: 1, Episode: i, Title: "Episode", Status: StatusWanted}
		if err := tx.AddEpisode(ep); err != nil {
			t.Fatalf("AddEpisode failed: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify all episodes exist
	eps, total, err := store.ListEpisodes(EpisodeFilter{ContentID: &series.ID})
	if err != nil {
		t.Fatalf("ListEpisodes failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 episodes, got %d", total)
	}
	if len(eps) != 3 {
		t.Errorf("expected 3 results, got %d", len(eps))
	}
}

func TestTx_CascadeDelete(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series with episode and file
	series := &Content{Type: ContentTypeSeries, Title: "Cascade Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("setup: %v", err)
	}
	ep := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted}
	if err := store.AddEpisode(ep); err != nil {
		t.Fatalf("setup: %v", err)
	}
	f := &File{ContentID: series.ID, EpisodeID: &ep.ID, Path: "/tv/series/s01e01.mkv", SizeBytes: 1000, Quality: "1080p", Source: "webdl"}
	if err := store.AddFile(f); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Delete content - should cascade
	if err := store.DeleteContent(series.ID); err != nil {
		t.Fatalf("DeleteContent failed: %v", err)
	}

	// Episode should be gone
	_, err := store.GetEpisode(ep.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected episode ErrNotFound after cascade, got %v", err)
	}

	// File should be gone
	_, err = store.GetFile(f.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected file ErrNotFound after cascade, got %v", err)
	}
}

func TestTx_CascadeDelete_InTransaction(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series with episode and file outside transaction
	series := &Content{Type: ContentTypeSeries, Title: "Cascade TX Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("setup AddContent: %v", err)
	}
	ep := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted}
	if err := store.AddEpisode(ep); err != nil {
		t.Fatalf("setup AddEpisode: %v", err)
	}
	f := &File{ContentID: series.ID, EpisodeID: &ep.ID, Path: "/tv/cascade/s01e01.mkv", SizeBytes: 1000, Quality: "1080p", Source: "webdl"}
	if err := store.AddFile(f); err != nil {
		t.Fatalf("setup AddFile: %v", err)
	}

	// Delete content in a transaction
	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	if err := tx.DeleteContent(series.ID); err != nil {
		t.Fatalf("tx.DeleteContent failed: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Episode should be gone after commit
	_, err = store.GetEpisode(ep.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected episode ErrNotFound after cascade delete, got %v", err)
	}

	// File should be gone after commit
	_, err = store.GetFile(f.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected file ErrNotFound after cascade delete, got %v", err)
	}
}

func TestTx_CascadeDelete_Rollback(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series with episode and file
	series := &Content{Type: ContentTypeSeries, Title: "Rollback Cascade Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("setup AddContent: %v", err)
	}
	ep := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted}
	if err := store.AddEpisode(ep); err != nil {
		t.Fatalf("setup AddEpisode: %v", err)
	}
	f := &File{ContentID: series.ID, EpisodeID: &ep.ID, Path: "/tv/rollback/s01e01.mkv", SizeBytes: 1000, Quality: "1080p", Source: "webdl"}
	if err := store.AddFile(f); err != nil {
		t.Fatalf("setup AddFile: %v", err)
	}

	// Delete content in a transaction, then rollback
	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	if err := tx.DeleteContent(series.ID); err != nil {
		t.Fatalf("tx.DeleteContent failed: %v", err)
	}

	// Rollback the delete
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Content should still exist
	_, err = store.GetContent(series.ID)
	if err != nil {
		t.Errorf("expected content to exist after rollback, got %v", err)
	}

	// Episode should still exist
	_, err = store.GetEpisode(ep.ID)
	if err != nil {
		t.Errorf("expected episode to exist after rollback, got %v", err)
	}

	// File should still exist
	_, err = store.GetFile(f.ID)
	if err != nil {
		t.Errorf("expected file to exist after rollback, got %v", err)
	}
}

func TestTx_EpisodeDelete_CascadeFiles(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series with episode and file
	series := &Content{Type: ContentTypeSeries, Title: "Episode Cascade Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := store.AddContent(series); err != nil {
		t.Fatalf("setup AddContent: %v", err)
	}
	ep := &Episode{ContentID: series.ID, Season: 1, Episode: 1, Title: "Pilot", Status: StatusWanted}
	if err := store.AddEpisode(ep); err != nil {
		t.Fatalf("setup AddEpisode: %v", err)
	}
	f := &File{ContentID: series.ID, EpisodeID: &ep.ID, Path: "/tv/episode-cascade/s01e01.mkv", SizeBytes: 1000, Quality: "1080p", Source: "webdl"}
	if err := store.AddFile(f); err != nil {
		t.Fatalf("setup AddFile: %v", err)
	}

	// Delete episode - file should cascade
	if err := store.DeleteEpisode(ep.ID); err != nil {
		t.Fatalf("DeleteEpisode failed: %v", err)
	}

	// Episode should be gone
	_, err := store.GetEpisode(ep.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected episode ErrNotFound after delete, got %v", err)
	}

	// File should be gone (cascade from episode)
	_, err = store.GetFile(f.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected file ErrNotFound after episode cascade delete, got %v", err)
	}

	// Series should still exist
	_, err = store.GetContent(series.ID)
	if err != nil {
		t.Errorf("expected series to still exist, got %v", err)
	}
}

func TestTx_MultipleEpisodesAndFiles(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create series with multiple episodes and files in a transaction
	tx, err := store.Begin()
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	series := &Content{Type: ContentTypeSeries, Title: "Multi Episode Series", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/tv"}
	if err := tx.AddContent(series); err != nil {
		t.Fatalf("tx.AddContent failed: %v", err)
	}

	var episodes []*Episode
	for i := 1; i <= 3; i++ {
		ep := &Episode{ContentID: series.ID, Season: 1, Episode: i, Title: "Episode", Status: StatusWanted}
		if err := tx.AddEpisode(ep); err != nil {
			t.Fatalf("tx.AddEpisode failed: %v", err)
		}
		episodes = append(episodes, ep)

		// Add a file for each episode
		f := &File{
			ContentID: series.ID,
			EpisodeID: &ep.ID,
			Path:      "/tv/multi/s01e0" + string(rune('0'+i)) + ".mkv",
			SizeBytes: 1000,
			Quality:   "1080p",
			Source:    "webdl",
		}
		if err := tx.AddFile(f); err != nil {
			t.Fatalf("tx.AddFile failed: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Verify all episodes exist
	_, total, err := store.ListEpisodes(EpisodeFilter{ContentID: &series.ID})
	if err != nil {
		t.Fatalf("ListEpisodes failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 episodes, got %d", total)
	}

	// Verify all files exist
	files, total, err := store.ListFiles(FileFilter{ContentID: &series.ID})
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 files, got %d", total)
	}
	if len(files) != 3 {
		t.Errorf("expected 3 file results, got %d", len(files))
	}

	// Delete series - all should cascade
	if err := store.DeleteContent(series.ID); err != nil {
		t.Fatalf("DeleteContent failed: %v", err)
	}

	// Verify all episodes are gone
	_, total, err = store.ListEpisodes(EpisodeFilter{ContentID: &series.ID})
	if err != nil {
		t.Fatalf("ListEpisodes after delete failed: %v", err)
	}
	if total != 0 {
		t.Errorf("expected 0 episodes after cascade delete, got %d", total)
	}

	// Verify all files are gone
	_, total, err = store.ListFiles(FileFilter{ContentID: &series.ID})
	if err != nil {
		t.Fatalf("ListFiles after delete failed: %v", err)
	}
	if total != 0 {
		t.Errorf("expected 0 files after cascade delete, got %d", total)
	}
}

func TestTx_MovieWithFile_CascadeDelete(t *testing.T) {
	db := setupTestDB(t)
	store := NewStore(db)

	// Create movie with file
	movie := &Content{Type: ContentTypeMovie, Title: "Cascade Movie", Year: 2024, Status: StatusWanted, QualityProfile: "hd", RootPath: "/movies"}
	if err := store.AddContent(movie); err != nil {
		t.Fatalf("setup AddContent: %v", err)
	}
	f := &File{ContentID: movie.ID, Path: "/movies/cascade-movie.mkv", SizeBytes: 5000, Quality: "1080p", Source: "usenet"}
	if err := store.AddFile(f); err != nil {
		t.Fatalf("setup AddFile: %v", err)
	}

	// Delete movie - file should cascade
	if err := store.DeleteContent(movie.ID); err != nil {
		t.Fatalf("DeleteContent failed: %v", err)
	}

	// File should be gone
	_, err := store.GetFile(f.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected file ErrNotFound after cascade delete, got %v", err)
	}
}
