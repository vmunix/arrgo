package release

import (
	"regexp"

	"github.com/hbollon/go-edlib"
)

// numberRegex extracts sequence numbers from titles (e.g., "2", "3")
var numberRegex = regexp.MustCompile(`\b(\d+)\b`)

// MatchConfidence represents the confidence level of a title match.
type MatchConfidence int

const (
	ConfidenceNone   MatchConfidence = iota // Score < 0.70
	ConfidenceLow                           // Score >= 0.70
	ConfidenceMedium                        // Score >= 0.85
	ConfidenceHigh                          // Score >= 0.95
)

func (c MatchConfidence) String() string {
	switch c {
	case ConfidenceHigh:
		return "high"
	case ConfidenceMedium:
		return "medium"
	case ConfidenceLow:
		return "low"
	default:
		return "none"
	}
}

// MatchResult represents the result of a fuzzy title match.
type MatchResult struct {
	Title      string          // The matched candidate title
	Score      float64         // Jaro-Winkler similarity score (0.0-1.0)
	Confidence MatchConfidence // Confidence level based on score
}

// MatchTitle finds the best match for a parsed title against candidate titles.
// Uses Jaro-Winkler similarity which favors prefix matches (good for media titles).
// Additionally applies a bonus when sequence numbers match between parsed and candidate.
// Returns the best match with confidence level based on score thresholds.
func MatchTitle(parsed string, candidates []string) MatchResult {
	if len(candidates) == 0 {
		return MatchResult{Confidence: ConfidenceNone}
	}

	// Normalize the parsed title for comparison
	normalizedParsed := CleanTitle(parsed)
	parsedNumbers := extractNumbers(normalizedParsed)

	best := MatchResult{
		Score:      0,
		Confidence: ConfidenceNone,
	}

	for _, candidate := range candidates {
		normalizedCandidate := CleanTitle(candidate)

		// Calculate Jaro-Winkler similarity (returns value between 0 and 1)
		score := float64(edlib.JaroWinklerSimilarity(normalizedParsed, normalizedCandidate))

		// Apply bonus/penalty for sequence number matching
		candidateNumbers := extractNumbers(normalizedCandidate)
		score = adjustScoreForNumbers(score, parsedNumbers, candidateNumbers)

		if score > best.Score {
			best.Title = candidate
			best.Score = score
		}
	}

	// Set confidence level based on score thresholds
	switch {
	case best.Score >= 0.95:
		best.Confidence = ConfidenceHigh
	case best.Score >= 0.85:
		best.Confidence = ConfidenceMedium
	case best.Score >= 0.70:
		best.Confidence = ConfidenceLow
	default:
		best.Confidence = ConfidenceNone
		best.Title = "" // Clear title for no-match case
	}

	return best
}

// extractNumbers returns all numeric sequences from a normalized title.
func extractNumbers(title string) []string {
	return numberRegex.FindAllString(title, -1)
}

// adjustScoreForNumbers modifies the similarity score based on sequence number matching.
// When the parsed title has numbers:
// - Matching numbers get a bonus
// - Mismatched numbers get a penalty
// - Missing numbers in candidate also get a penalty
func adjustScoreForNumbers(score float64, parsedNums, candidateNums []string) float64 {
	if len(parsedNums) == 0 {
		return score
	}

	// Check if numbers match
	parsedSet := make(map[string]bool)
	for _, n := range parsedNums {
		parsedSet[n] = true
	}

	candidateSet := make(map[string]bool)
	for _, n := range candidateNums {
		candidateSet[n] = true
	}

	// If parsed has numbers but candidate doesn't, apply penalty
	if len(candidateNums) == 0 && len(parsedNums) > 0 {
		return score * 0.85
	}

	// Check for number matches
	matchFound := false
	for n := range parsedSet {
		if candidateSet[n] {
			matchFound = true
			break
		}
	}

	if matchFound {
		// Bonus for matching sequence number, capped at 1.0
		return min(score*1.05, 1.0)
	}

	// Penalty for mismatched numbers
	return score * 0.90
}
