package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/vmunix/arrgo/pkg/release"
)

const (
	contentTypeMovie  = "movie"
	contentTypeSeries = "series"
)

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func prompt(message string) string {
	fmt.Print(message)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

// buildBadges creates a formatted string of quality badges from parsed release info.
// Badge order: resolution, source, codec, HDR, audio, remux, edition
// Only includes badges for detected (non-unknown/non-empty) attributes.
func buildBadges(info *release.Info) string {
	var badges []string

	// Resolution
	if info.Resolution != release.ResolutionUnknown {
		badges = append(badges, "["+info.Resolution.String()+"]")
	}

	// Source - format nicely
	if info.Source != release.SourceUnknown {
		source := info.Source.String()
		// Capitalize nicely
		switch info.Source {
		case release.SourceBluRay:
			source = "BluRay"
		case release.SourceWEBDL:
			source = "WEB-DL"
		case release.SourceWEBRip:
			source = "WEBRip"
		case release.SourceHDTV:
			source = "HDTV"
		}
		badges = append(badges, "["+source+"]")
	}

	// Codec
	if info.Codec != release.CodecUnknown {
		badges = append(badges, "["+info.Codec.String()+"]")
	}

	// HDR
	if info.HDR != release.HDRNone {
		badges = append(badges, "["+info.HDR.String()+"]")
	}

	// Audio
	if info.Audio != release.AudioUnknown {
		badges = append(badges, "["+info.Audio.String()+"]")
	}

	// Remux
	if info.IsRemux {
		badges = append(badges, "[Remux]")
	}

	// Edition
	if info.Edition != "" {
		badges = append(badges, "["+info.Edition+"]")
	}

	return strings.Join(badges, " ")
}

func grabRelease(client *Client, rel ReleaseResponse, contentType, profile string) {
	// Parse release name to get title/year
	info := release.Parse(rel.Title)
	if info.Title == "" {
		fmt.Fprintln(os.Stderr, "Error: Could not parse release title")
		return
	}

	// Determine content type if not specified
	if contentType == "" {
		if info.Season > 0 || info.Episode > 0 {
			contentType = contentTypeSeries
		} else {
			contentType = contentTypeMovie
		}
	}

	// Show what we parsed
	fmt.Printf("\nParsed: %s (%d)\n", info.Title, info.Year)

	input := prompt("Confirm? [Y/n]: ")
	if input != "" && input != "y" && input != "Y" {
		fmt.Println("Canceled")
		return
	}

	// Default profile
	if profile == "" {
		profile = "hd"
	}

	// Try to find existing content first
	content, err := client.FindContent(contentType, info.Title, info.Year)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding content: %v\n", err)
		return
	}

	if content != nil {
		fmt.Printf("Found in library (ID: %d)\n", content.ID)
	} else {
		// Create new content entry
		content, err = client.AddContent(contentType, info.Title, info.Year, profile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error adding content: %v\n", err)
			return
		}
		fmt.Printf("Added to library (ID: %d)\n", content.ID)
	}

	// Grab the release
	grab, err := client.Grab(content.ID, rel.DownloadURL, rel.Title, rel.Indexer)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error grabbing: %v\n", err)
		return
	}
	fmt.Printf("Download started (ID: %d)\n", grab.DownloadID)
}
