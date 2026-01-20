package release

import (
	"encoding/csv"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_Corpus(t *testing.T) {
	f, err := os.Open("../../testdata/releases.csv")
	if err != nil {
		t.Skipf("corpus not found: %v", err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	require.NoError(t, err, "read csv")

	var stats struct {
		total         int
		emptyTitle    int
		hasResolution int
		hasSource     int
		hasCodec      int
		hasYear       int
		hasGroup      int
		// New
		hasHDR       int
		hasAudio     int
		hasRemux     int
		hasEdition   int
		hasService   int
		hasDailyDate int
	}

	for i, rec := range records {
		if i == 0 {
			continue // skip header
		}
		title := rec[0]
		stats.total++

		info := Parse(title)

		if info.Title == "" {
			stats.emptyTitle++
		}
		if info.Resolution != ResolutionUnknown {
			stats.hasResolution++
		}
		if info.Source != SourceUnknown {
			stats.hasSource++
		}
		if info.Codec != CodecUnknown {
			stats.hasCodec++
		}
		if info.Year > 0 {
			stats.hasYear++
		}
		if info.Group != "" {
			stats.hasGroup++
		}
		if info.HDR != HDRNone {
			stats.hasHDR++
		}
		if info.Audio != AudioUnknown {
			stats.hasAudio++
		}
		if info.IsRemux {
			stats.hasRemux++
		}
		if info.Edition != "" {
			stats.hasEdition++
		}
		if info.Service != "" {
			stats.hasService++
		}
		if info.DailyDate != "" {
			stats.hasDailyDate++
		}
	}

	t.Logf("Corpus Stats:")
	t.Logf("  Total:          %d", stats.total)
	t.Logf("  Empty Title:    %d (%.1f%%)", stats.emptyTitle, pct(stats.emptyTitle, stats.total))
	t.Logf("  Has Resolution: %d (%.1f%%)", stats.hasResolution, pct(stats.hasResolution, stats.total))
	t.Logf("  Has Source:     %d (%.1f%%)", stats.hasSource, pct(stats.hasSource, stats.total))
	t.Logf("  Has Codec:      %d (%.1f%%)", stats.hasCodec, pct(stats.hasCodec, stats.total))
	t.Logf("  Has Year:       %d (%.1f%%)", stats.hasYear, pct(stats.hasYear, stats.total))
	t.Logf("  Has Group:      %d (%.1f%%)", stats.hasGroup, pct(stats.hasGroup, stats.total))
	t.Logf("  Has HDR:        %d (%.1f%%)", stats.hasHDR, pct(stats.hasHDR, stats.total))
	t.Logf("  Has Audio:      %d (%.1f%%)", stats.hasAudio, pct(stats.hasAudio, stats.total))
	t.Logf("  Has Remux:      %d (%.1f%%)", stats.hasRemux, pct(stats.hasRemux, stats.total))
	t.Logf("  Has Edition:    %d (%.1f%%)", stats.hasEdition, pct(stats.hasEdition, stats.total))
	t.Logf("  Has Service:    %d (%.1f%%)", stats.hasService, pct(stats.hasService, stats.total))
	t.Logf("  Has DailyDate:  %d (%.1f%%)", stats.hasDailyDate, pct(stats.hasDailyDate, stats.total))

	// Updated thresholds based on improvements
	emptyPct := pct(stats.emptyTitle, stats.total)
	assert.LessOrEqual(t, emptyPct, 5.0, "Too many empty titles: %.1f%% (want < 5%%)", emptyPct)

	resPct := pct(stats.hasResolution, stats.total)
	assert.GreaterOrEqual(t, resPct, 90.0, "Resolution detection too low: %.1f%% (want > 90%%)", resPct)

	codecPct := pct(stats.hasCodec, stats.total)
	assert.GreaterOrEqual(t, codecPct, 60.0, "Codec detection too low: %.1f%% (want > 60%%)", codecPct)
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}

func BenchmarkParse_Corpus(b *testing.B) {
	f, err := os.Open("../../testdata/releases.csv")
	if err != nil {
		b.Skipf("corpus not found: %v", err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		b.Fatalf("read csv: %v", err)
	}

	titles := make([]string, 0, len(records)-1)
	for i, rec := range records {
		if i == 0 {
			continue
		}
		titles = append(titles, rec[0])
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, title := range titles {
			Parse(title)
		}
	}
}
