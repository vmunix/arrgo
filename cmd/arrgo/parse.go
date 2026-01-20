package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vmunix/arrgo/internal/config"
	"github.com/vmunix/arrgo/pkg/release"
)

// ScoreBonus represents a single bonus in the score breakdown.
type ScoreBonus struct {
	Attribute string `json:"attribute"`
	Value     string `json:"value"`
	Position  int    `json:"position"` // 0-indexed, -1 if not from preference list
	Bonus     int    `json:"bonus"`
	Note      string `json:"note,omitempty"`
}

// ParseResult contains the parsed release info and optional score breakdown.
type ParseResult struct {
	Info      *release.Info
	Score     int
	Profile   string
	Breakdown []ScoreBonus
}

// ParseResultJSON is the JSON-friendly representation of ParseResult.
type ParseResultJSON struct {
	Title      string       `json:"title"`
	Year       int          `json:"year,omitempty"`
	Season     int          `json:"season,omitempty"`
	Episode    int          `json:"episode,omitempty"`
	DailyDate  string       `json:"daily_date,omitempty"`
	Resolution string       `json:"resolution"`
	Source     string       `json:"source"`
	Codec      string       `json:"codec"`
	HDR        string       `json:"hdr,omitempty"`
	Audio      string       `json:"audio,omitempty"`
	IsRemux    bool         `json:"remux"`
	Edition    string       `json:"edition,omitempty"`
	Service    string       `json:"service,omitempty"`
	Group      string       `json:"group,omitempty"`
	Proper     bool         `json:"proper,omitempty"`
	Repack     bool         `json:"repack,omitempty"`
	CleanTitle string       `json:"clean_title"`
	Score      int          `json:"score,omitempty"`
	Profile    string       `json:"profile,omitempty"`
	Breakdown  []ScoreBonus `json:"breakdown,omitempty"`
}

// toJSON converts ParseResult to its JSON-friendly representation.
func (r ParseResult) toJSON() ParseResultJSON {
	info := r.Info
	result := ParseResultJSON{
		Title:      info.Title,
		Year:       info.Year,
		Season:     info.Season,
		Episode:    info.Episode,
		DailyDate:  info.DailyDate,
		Resolution: info.Resolution.String(),
		Source:     info.Source.String(),
		Codec:      info.Codec.String(),
		IsRemux:    info.IsRemux,
		Edition:    info.Edition,
		Service:    info.Service,
		Group:      info.Group,
		Proper:     info.Proper,
		Repack:     info.Repack,
		CleanTitle: info.CleanTitle,
		Score:      r.Score,
		Profile:    r.Profile,
		Breakdown:  r.Breakdown,
	}

	// Set HDR only if present
	if info.HDR != release.HDRNone {
		result.HDR = info.HDR.String()
	}

	// Set Audio only if present
	if info.Audio != release.AudioUnknown {
		result.Audio = info.Audio.String()
	}

	return result
}

var parseCmd = &cobra.Command{
	Use:   "parse [flags] <release-name>",
	Short: "Parse release name (local, no server needed)",
	Long: `Parse a release name to extract metadata.

Examples:
  arrgo parse "The.Matrix.1999.2160p.UHD.BluRay.x265-GROUP"
  arrgo parse --score hd "Movie.2024.1080p.WEB-DL.x264-GROUP"
  arrgo parse --file releases.txt --json`,
	RunE: runParseCmd,
}

func init() {
	rootCmd.AddCommand(parseCmd)
	parseCmd.Flags().String("score", "", "Score against quality profile")
	parseCmd.Flags().StringP("file", "f", "", "Read release names from file (one per line)")
	parseCmd.Flags().String("config", "config.toml", "Path to config file")
	// Note: --json is inherited from root as persistent flag
}

func runParseCmd(cmd *cobra.Command, args []string) error {
	scoreProfile, _ := cmd.Flags().GetString("score")
	inputFile, _ := cmd.Flags().GetString("file")
	configPath, _ := cmd.Flags().GetString("config")

	// Determine input mode
	var releaseNames []string
	if inputFile != "" {
		// Batch mode: read from file
		names, err := readReleaseFile(inputFile)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}
		releaseNames = names
	} else if len(args) > 0 {
		// Single release from command line
		releaseNames = []string{args[0]}
	} else {
		return fmt.Errorf("usage: arrgo parse <release-name> or arrgo parse --file <filename>")
	}

	// Load config if scoring is requested
	var cfg *config.Config
	if scoreProfile != "" {
		var err error
		cfg, err = config.LoadWithoutValidation(configPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		// Verify profile exists
		if _, ok := cfg.Quality.Profiles[scoreProfile]; !ok {
			return fmt.Errorf("profile '%s' not found. Available: %s",
				scoreProfile, strings.Join(getProfileNames(cfg), ", "))
		}
	}

	// Parse all releases
	results := make([]ParseResult, 0, len(releaseNames))
	for _, name := range releaseNames {
		info := release.Parse(name)
		result := ParseResult{Info: info}

		if scoreProfile != "" && cfg != nil {
			profile := cfg.Quality.Profiles[scoreProfile]
			result.Profile = scoreProfile
			result.Score, result.Breakdown = scoreWithBreakdown(*info, profile)
		}

		results = append(results, result)
	}

	// Output results (use global jsonOutput from root.go)
	if jsonOutput {
		outputJSON(results)
	} else {
		for i, result := range results {
			if i > 0 {
				fmt.Println()
			}
			printHumanReadable(result)
		}
	}
	return nil
}

// readReleaseFile reads release names from a file, one per line.
func readReleaseFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var names []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			names = append(names, line)
		}
	}
	return names, scanner.Err()
}

// getProfileNames returns a list of available profile names.
func getProfileNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Quality.Profiles))
	for name := range cfg.Quality.Profiles {
		names = append(names, name)
	}
	return names
}

// Score constants (matching internal/search/scorer.go)
const (
	scoreResolution2160p = 100
	scoreResolution1080p = 80
	scoreResolution720p  = 60
	scoreResolutionOther = 40
	bonusSource          = 10
	bonusCodec           = 10
	bonusHDR             = 15
	bonusAudio           = 15
	bonusRemux           = 20
)

// scoreWithBreakdown calculates the score and returns a detailed breakdown.
func scoreWithBreakdown(info release.Info, profile config.QualityProfile) (int, []ScoreBonus) {
	// Check reject list first
	if matchesRejectList(info, profile.Reject) {
		return 0, []ScoreBonus{{
			Attribute: "Reject",
			Value:     "matched reject list",
			Position:  -1,
			Bonus:     0,
			Note:      "release rejected",
		}}
	}

	var breakdown []ScoreBonus
	totalScore := 0

	// Check resolution
	resScore, resBonus := scoreResolution(info.Resolution, profile.Resolution)
	if resBonus.Bonus > 0 || resScore > 0 {
		breakdown = append(breakdown, resBonus)
		totalScore += resScore
	}
	if resScore == 0 && len(profile.Resolution) > 0 {
		// Resolution not matched, return 0
		return 0, []ScoreBonus{{
			Attribute: "Resolution",
			Value:     info.Resolution.String(),
			Position:  -1,
			Bonus:     0,
			Note:      "not in allowed list",
		}}
	}

	// Source bonus
	if bonus := scoreAttribute(info.Source.String(), profile.Sources, bonusSource, "Source"); bonus.Bonus > 0 || info.Source != release.SourceUnknown {
		if bonus.Value != "unknown" {
			breakdown = append(breakdown, bonus)
			totalScore += bonus.Bonus
		}
	}

	// Codec bonus
	if bonus := scoreAttribute(info.Codec.String(), profile.Codecs, bonusCodec, "Codec"); bonus.Bonus > 0 || info.Codec != release.CodecUnknown {
		if bonus.Value != "unknown" {
			breakdown = append(breakdown, bonus)
			totalScore += bonus.Bonus
		}
	}

	// HDR bonus
	if info.HDR != release.HDRNone {
		bonus := scoreHDR(info.HDR, profile.HDR)
		breakdown = append(breakdown, bonus)
		totalScore += bonus.Bonus
	}

	// Audio bonus
	if info.Audio != release.AudioUnknown {
		bonus := scoreAudioCodec(info.Audio, profile.Audio)
		breakdown = append(breakdown, bonus)
		totalScore += bonus.Bonus
	}

	// Remux bonus
	if info.IsRemux && profile.PreferRemux {
		breakdown = append(breakdown, ScoreBonus{
			Attribute: "Remux",
			Value:     "yes",
			Position:  -1,
			Bonus:     bonusRemux,
			Note:      "preferred",
		})
		totalScore += bonusRemux
	}

	return totalScore, breakdown
}

// scoreResolution returns the base resolution score and breakdown entry.
func scoreResolution(res release.Resolution, preferences []string) (int, ScoreBonus) {
	resStr := res.String()
	baseScore := resolutionBaseScore(res)

	bonus := ScoreBonus{
		Attribute: "Resolution",
		Value:     resStr,
		Position:  -1,
		Bonus:     baseScore,
	}

	// If no preference list, accept any resolution
	if len(preferences) == 0 {
		bonus.Note = "no restrictions"
		return baseScore, bonus
	}

	// Check if resolution is in preference list
	for i, pref := range preferences {
		if strings.EqualFold(resStr, pref) {
			bonus.Position = i
			bonus.Note = fmt.Sprintf("#%d choice", i+1)
			return baseScore, bonus
		}
	}

	// Not in allowed list
	bonus.Bonus = 0
	bonus.Note = "not in allowed list"
	return 0, bonus
}

// resolutionBaseScore returns the base score for a resolution.
func resolutionBaseScore(r release.Resolution) int {
	switch r {
	case release.Resolution2160p:
		return scoreResolution2160p
	case release.Resolution1080p:
		return scoreResolution1080p
	case release.Resolution720p:
		return scoreResolution720p
	default:
		return scoreResolutionOther
	}
}

// scoreAttribute calculates position-based bonus for a generic attribute.
func scoreAttribute(value string, preferences []string, baseBonus int, attrName string) ScoreBonus {
	bonus := ScoreBonus{
		Attribute: attrName,
		Value:     value,
		Position:  -1,
		Bonus:     0,
	}

	if value == "" || value == "unknown" {
		return bonus
	}

	if len(preferences) == 0 {
		return bonus
	}

	valueLower := strings.ToLower(value)
	for i, pref := range preferences {
		if strings.EqualFold(valueLower, pref) {
			multiplier := 1.0 - 0.2*float64(i)
			if multiplier < 0 {
				multiplier = 0
			}
			bonus.Position = i
			bonus.Bonus = int(float64(baseBonus) * multiplier)
			bonus.Note = fmt.Sprintf("#%d choice", i+1)
			return bonus
		}
	}

	bonus.Note = "not in preference list"
	return bonus
}

// scoreHDR calculates HDR bonus with position awareness.
func scoreHDR(hdr release.HDRFormat, preferences []string) ScoreBonus {
	bonus := ScoreBonus{
		Attribute: "HDR",
		Value:     hdr.String(),
		Position:  -1,
		Bonus:     0,
	}

	if len(preferences) == 0 {
		return bonus
	}

	for i, pref := range preferences {
		if hdrMatches(hdr, pref) {
			multiplier := 1.0 - 0.2*float64(i)
			if multiplier < 0 {
				multiplier = 0
			}
			bonus.Position = i
			bonus.Bonus = int(float64(bonusHDR) * multiplier)
			bonus.Note = fmt.Sprintf("#%d choice", i+1)
			return bonus
		}
	}

	bonus.Note = "not in preference list"
	return bonus
}

// hdrMatches checks if an HDR format matches a preference string.
func hdrMatches(hdr release.HDRFormat, pref string) bool {
	prefLower := strings.ToLower(pref)
	switch hdr {
	case release.DolbyVision:
		return prefLower == "dolby-vision" || prefLower == "dv" || prefLower == "dolbyvision"
	case release.HDR10Plus:
		return prefLower == "hdr10+" || prefLower == "hdr10plus"
	case release.HDR10:
		return prefLower == "hdr10"
	case release.HDRGeneric:
		return prefLower == "hdr"
	case release.HLG:
		return prefLower == "hlg"
	default:
		return false
	}
}

// scoreAudioCodec calculates audio bonus with position awareness.
func scoreAudioCodec(audio release.AudioCodec, preferences []string) ScoreBonus {
	bonus := ScoreBonus{
		Attribute: "Audio",
		Value:     audio.String(),
		Position:  -1,
		Bonus:     0,
	}

	if len(preferences) == 0 {
		return bonus
	}

	for i, pref := range preferences {
		if audioMatches(audio, pref) {
			multiplier := 1.0 - 0.2*float64(i)
			if multiplier < 0 {
				multiplier = 0
			}
			bonus.Position = i
			bonus.Bonus = int(float64(bonusAudio) * multiplier)
			bonus.Note = fmt.Sprintf("#%d choice", i+1)
			return bonus
		}
	}

	bonus.Note = "not in preference list"
	return bonus
}

// audioMatches checks if an audio codec matches a preference string.
func audioMatches(audio release.AudioCodec, pref string) bool {
	prefLower := strings.ToLower(pref)
	switch audio {
	case release.AudioAtmos:
		return prefLower == "atmos"
	case release.AudioTrueHD:
		return prefLower == "truehd"
	case release.AudioDTSHD:
		return prefLower == "dtshd" || prefLower == "dts-hd" || prefLower == "dts-hd ma"
	case release.AudioDTS:
		return prefLower == "dts"
	case release.AudioEAC3:
		return prefLower == "dd+" || prefLower == "ddp" || prefLower == "eac3"
	case release.AudioAC3:
		return prefLower == "dd" || prefLower == "ac3"
	case release.AudioAAC:
		return prefLower == "aac"
	case release.AudioFLAC:
		return prefLower == "flac"
	case release.AudioOpus:
		return prefLower == "opus"
	default:
		return false
	}
}

// matchesRejectList checks if a release matches any reject criteria.
func matchesRejectList(info release.Info, rejectList []string) bool {
	if len(rejectList) == 0 {
		return false
	}

	// Build lowercase set of release attributes
	attrs := []string{
		strings.ToLower(info.Resolution.String()),
		strings.ToLower(info.Source.String()),
		strings.ToLower(info.Codec.String()),
	}

	// Add HDR format if present
	if info.HDR != release.HDRNone {
		attrs = append(attrs, strings.ToLower(info.HDR.String()))
	}

	// Add audio codec if present
	if info.Audio != release.AudioUnknown {
		attrs = append(attrs, strings.ToLower(info.Audio.String()))
	}

	// Check each reject term
	for _, reject := range rejectList {
		rejectLower := strings.ToLower(reject)
		for _, attr := range attrs {
			if attr == rejectLower {
				return true
			}
		}
		// Also check special cases for reject list
		if rejectMatchesSpecial(info, rejectLower) {
			return true
		}
	}

	return false
}

// rejectMatchesSpecial handles special reject list matching.
func rejectMatchesSpecial(info release.Info, reject string) bool {
	switch reject {
	case "cam", "camrip", "ts", "telesync", "hdcam":
		// Low-quality sources not currently tracked by parser
		return false
	case "hdtv":
		return info.Source == release.SourceHDTV
	case "webrip":
		return info.Source == release.SourceWEBRip
	case "remux":
		return info.IsRemux
	case "x264", "h264":
		return info.Codec == release.CodecX264
	case "x265", "h265", "hevc":
		return info.Codec == release.CodecX265
	}
	return false
}

// printHumanReadable outputs the parse result in a human-readable format.
func printHumanReadable(result ParseResult) {
	info := result.Info

	fmt.Printf("Title:       %s\n", valueOrEmpty(info.Title))
	if info.Year > 0 {
		fmt.Printf("Year:        %d\n", info.Year)
	}
	if info.Season > 0 || info.Episode > 0 {
		fmt.Printf("Season:      %d\n", info.Season)
		fmt.Printf("Episode:     %d\n", info.Episode)
	}
	if info.DailyDate != "" {
		fmt.Printf("Date:        %s\n", info.DailyDate)
	}
	fmt.Printf("Resolution:  %s\n", info.Resolution.String())
	fmt.Printf("Source:      %s\n", sourceDisplayName(info.Source))
	fmt.Printf("Codec:       %s\n", info.Codec.String())
	if info.HDR != release.HDRNone {
		fmt.Printf("HDR:         %s\n", hdrDisplayName(info.HDR))
	}
	if info.Audio != release.AudioUnknown {
		fmt.Printf("Audio:       %s\n", info.Audio.String())
	}
	fmt.Printf("Remux:       %s\n", boolToYesNo(info.IsRemux))
	if info.Edition != "" {
		fmt.Printf("Edition:     %s\n", info.Edition)
	}
	if info.Service != "" {
		fmt.Printf("Service:     %s\n", info.Service)
	}
	if info.Group != "" {
		fmt.Printf("Group:       %s\n", info.Group)
	}
	if info.Proper {
		fmt.Printf("Proper:      yes\n")
	}
	if info.Repack {
		fmt.Printf("Repack:      yes\n")
	}
	fmt.Printf("CleanTitle:  %s\n", valueOrEmpty(info.CleanTitle))

	// Print score breakdown if available
	if result.Profile != "" && len(result.Breakdown) > 0 {
		fmt.Println()
		fmt.Printf("Score Breakdown (profile: %s):\n", result.Profile)
		for _, b := range result.Breakdown {
			note := ""
			if b.Note != "" {
				note = fmt.Sprintf(", %s", b.Note)
			}
			fmt.Printf("  %-12s (%s%s):  %+d\n", b.Attribute, b.Value, note, b.Bonus)
		}
		fmt.Println("  " + strings.Repeat("\u2500", 37))
		fmt.Printf("  Total:                           %d\n", result.Score)
	}
}

// sourceDisplayName returns a display-friendly source name.
func sourceDisplayName(s release.Source) string {
	switch s {
	case release.SourceBluRay:
		return "BluRay"
	case release.SourceWEBDL:
		return "WEB-DL"
	case release.SourceWEBRip:
		return "WEBRip"
	case release.SourceHDTV:
		return "HDTV"
	default:
		return "unknown"
	}
}

// hdrDisplayName returns a display-friendly HDR format name.
func hdrDisplayName(h release.HDRFormat) string {
	switch h {
	case release.DolbyVision:
		return "Dolby Vision"
	case release.HDR10Plus:
		return "HDR10+"
	case release.HDR10:
		return "HDR10"
	case release.HDRGeneric:
		return "HDR"
	case release.HLG:
		return "HLG"
	default:
		return ""
	}
}

// valueOrEmpty returns the value or an empty placeholder.
func valueOrEmpty(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// boolToYesNo converts a boolean to yes/no string.
func boolToYesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// outputJSON outputs results as JSON.
func outputJSON(results []ParseResult) {
	// Convert to JSON-friendly format
	jsonResults := make([]ParseResultJSON, len(results))
	for i, r := range results {
		jsonResults[i] = r.toJSON()
	}

	// For single result, output object; for multiple, output array
	var output interface{}
	if len(jsonResults) == 1 {
		output = jsonResults[0]
	} else {
		output = jsonResults
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}
