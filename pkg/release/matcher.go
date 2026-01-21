package release

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
