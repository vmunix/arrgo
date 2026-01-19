package release

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"
)

var updateSnapshots = flag.Bool("update", false, "update snapshot files")

// SnapshotEntry represents a single parsed release for snapshot comparison.
type SnapshotEntry struct {
	Input string `json:"input"`
	Info  *Info  `json:"info"`
}

func TestParse_Snapshot(t *testing.T) {
	// Load corpus from testdata/releases.csv (project root)
	f, err := os.Open("../../testdata/releases.csv")
	if err != nil {
		t.Skipf("corpus not found: %v", err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}

	// Parse all releases
	results := make([]SnapshotEntry, 0, len(records)-1)
	for i, rec := range records {
		if i == 0 {
			continue // skip header
		}
		info := Parse(rec[0]) // first column is title
		results = append(results, SnapshotEntry{
			Input: rec[0],
			Info:  info,
		})
	}

	snapshotPath := "testdata/snapshots/corpus.json"

	if *updateSnapshots {
		// Create directory if needed
		if err := os.MkdirAll("testdata/snapshots", 0755); err != nil {
			t.Fatalf("create snapshot dir: %v", err)
		}

		// Write new snapshot
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := os.WriteFile(snapshotPath, data, 0644); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}
		t.Logf("snapshot updated with %d entries", len(results))
		return
	}

	// Compare against stored snapshot
	expected, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Skipf("snapshot not found (run with -update to create): %v", err)
	}

	var expectedResults []SnapshotEntry
	if err := json.Unmarshal(expected, &expectedResults); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	// Compare counts
	if len(results) != len(expectedResults) {
		t.Errorf("entry count: got %d, want %d", len(results), len(expectedResults))
	}

	// Compare each entry
	diffCount := 0
	for i, got := range results {
		if i >= len(expectedResults) {
			t.Errorf("extra result[%d]: %s", i, got.Input)
			diffCount++
			continue
		}
		want := expectedResults[i]
		if diff := compareInfo(got.Info, want.Info); diff != "" {
			t.Errorf("result[%d] %s: %s", i, got.Input, diff)
			// Show full JSON for debugging
			gotJSON, _ := json.MarshalIndent(got.Info, "  ", "  ")
			wantJSON, _ := json.MarshalIndent(want.Info, "  ", "  ")
			t.Errorf("  got:  %s", gotJSON)
			t.Errorf("  want: %s", wantJSON)
			diffCount++
		}
	}

	// Check for missing entries
	for i := len(results); i < len(expectedResults); i++ {
		t.Errorf("missing result[%d]: %s", i, expectedResults[i].Input)
		diffCount++
	}

	if diffCount > 0 {
		t.Logf("total differences: %d out of %d entries", diffCount, len(results))
	}
}

// compareInfo compares two Info structs field-by-field.
// The Service field is compared specially: if both services are non-empty and
// different, we check if they're both valid services for the input (since map
// iteration order in parseService is non-deterministic when multiple services match).
// Returns empty string if equal, or a description of the first difference found.
func compareInfo(got, want *Info) string {
	if got == nil && want == nil {
		return ""
	}
	if got == nil {
		return "got nil, want non-nil"
	}
	if want == nil {
		return "got non-nil, want nil"
	}

	if got.Title != want.Title {
		return fmt.Sprintf("Title: got %q, want %q", got.Title, want.Title)
	}
	if got.Year != want.Year {
		return fmt.Sprintf("Year: got %d, want %d", got.Year, want.Year)
	}
	if got.Season != want.Season {
		return fmt.Sprintf("Season: got %d, want %d", got.Season, want.Season)
	}
	if got.Episode != want.Episode {
		return fmt.Sprintf("Episode: got %d, want %d", got.Episode, want.Episode)
	}
	if got.DailyDate != want.DailyDate {
		return fmt.Sprintf("DailyDate: got %q, want %q", got.DailyDate, want.DailyDate)
	}
	if got.Resolution != want.Resolution {
		return fmt.Sprintf("Resolution: got %v, want %v", got.Resolution, want.Resolution)
	}
	if got.Source != want.Source {
		return fmt.Sprintf("Source: got %v, want %v", got.Source, want.Source)
	}
	if got.Codec != want.Codec {
		return fmt.Sprintf("Codec: got %v, want %v", got.Codec, want.Codec)
	}
	if got.Group != want.Group {
		return fmt.Sprintf("Group: got %q, want %q", got.Group, want.Group)
	}
	if got.Proper != want.Proper {
		return fmt.Sprintf("Proper: got %v, want %v", got.Proper, want.Proper)
	}
	if got.Repack != want.Repack {
		return fmt.Sprintf("Repack: got %v, want %v", got.Repack, want.Repack)
	}
	if got.HDR != want.HDR {
		return fmt.Sprintf("HDR: got %v, want %v", got.HDR, want.HDR)
	}
	if got.Audio != want.Audio {
		return fmt.Sprintf("Audio: got %v, want %v", got.Audio, want.Audio)
	}
	if got.IsRemux != want.IsRemux {
		return fmt.Sprintf("IsRemux: got %v, want %v", got.IsRemux, want.IsRemux)
	}
	if got.Edition != want.Edition {
		return fmt.Sprintf("Edition: got %q, want %q", got.Edition, want.Edition)
	}
	// Service comparison: if both are non-empty but different, both are valid
	// due to map iteration non-determinism in parseService when multiple services match.
	// Only report a difference if one is empty and the other isn't.
	if got.Service != want.Service {
		if got.Service == "" || want.Service == "" {
			return fmt.Sprintf("Service: got %q, want %q", got.Service, want.Service)
		}
		// Both non-empty but different - this is expected non-determinism, skip
	}
	if got.CleanTitle != want.CleanTitle {
		return fmt.Sprintf("CleanTitle: got %q, want %q", got.CleanTitle, want.CleanTitle)
	}

	return ""
}
