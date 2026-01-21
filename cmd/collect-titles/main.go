// Command collect-titles fetches release titles from configured indexers
// for use in building test suites for release name parsing.
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/vmunix/arrgo/internal/config"
	"github.com/vmunix/arrgo/pkg/newznab"
)

func main() {
	configPath := flag.String("config", "config.toml", "Path to config file")
	output := flag.String("output", "testdata/releases.csv", "Output CSV file")
	pagesPerCategory := flag.Int("pages", 10, "Pages to fetch per category per indexer")
	limit := flag.Int("limit", 100, "Results per page")
	flag.Parse()

	if err := run(*configPath, *output, *pagesPerCategory, *limit); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(configPath, output string, pages, limit int) error {
	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if len(cfg.Indexers) == 0 {
		return fmt.Errorf("no indexers configured")
	}

	// Create clients
	clients := make([]*newznab.Client, 0, len(cfg.Indexers))
	for name, idx := range cfg.Indexers {
		clients = append(clients, newznab.NewClient(name, idx.URL, idx.APIKey, nil))
	}

	// Categories to fetch
	categories := []struct {
		name string
		cats []int
	}{
		{"movie", []int{2000, 2010, 2020, 2030, 2040, 2045, 2050}},
		{"series", []int{5000, 5010, 5020, 5030, 5040, 5045, 5050, 5070}},
	}

	// Dedupe by title
	seen := make(map[string]bool)
	var results []record

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	for _, client := range clients {
		fmt.Printf("Fetching from %s...\n", client.Name())

		for _, cat := range categories {
			for page := 0; page < pages; page++ {
				offset := page * limit

				releases, err := client.SearchWithOffset(ctx, "", cat.cats, limit, offset)
				if err != nil {
					fmt.Printf("  %s page %d: error: %v\n", cat.name, page, err)
					continue
				}

				newCount := 0
				for _, rel := range releases {
					if seen[rel.Title] {
						continue
					}
					seen[rel.Title] = true
					newCount++

					results = append(results, record{
						Title:    rel.Title,
						Size:     rel.Size,
						Category: cat.name,
						Indexer:  client.Name(),
					})
				}

				fmt.Printf("  %s page %d: %d results, %d new\n", cat.name, page+1, len(releases), newCount)

				if len(releases) < limit {
					break // No more results
				}

				// Be nice to indexers
				time.Sleep(500 * time.Millisecond)
			}
		}
	}

	fmt.Printf("\nTotal unique titles: %d\n", len(results))

	// Write CSV
	if err := writeCSV(output, results); err != nil {
		return fmt.Errorf("write csv: %w", err)
	}

	fmt.Printf("Written to %s\n", output)
	return nil
}

type record struct {
	Title    string
	Size     int64
	Category string
	Indexer  string
}

func writeCSV(path string, records []record) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Header
	if err := w.Write([]string{"title", "size", "category", "indexer"}); err != nil {
		return err
	}

	// Data
	for _, r := range records {
		if err := w.Write([]string{
			r.Title,
			fmt.Sprintf("%d", r.Size),
			r.Category,
			r.Indexer,
		}); err != nil {
			return err
		}
	}

	return w.Error()
}
