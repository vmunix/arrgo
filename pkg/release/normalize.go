package release

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// CleanTitle normalizes a title for matching purposes.
// Removes articles, punctuation, accents, and normalizes whitespace.
func CleanTitle(title string) string {
	s := strings.ToLower(title)

	// Remove accents
	s = removeAccents(s)

	// Normalize punctuation
	s = strings.ReplaceAll(s, "&", " and ")
	s = strings.ReplaceAll(s, "-", " ")
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, ".", " ")

	// Split on colon to handle subtitles (e.g., "Léon: The Professional")
	// Strip leading articles from each part
	parts := strings.Split(s, ":")
	for i, part := range parts {
		parts[i] = stripLeadingArticle(strings.TrimSpace(part))
	}
	s = strings.Join(parts, " ")

	// Remove other punctuation
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	s = b.String()

	// Collapse whitespace
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

func removeAccents(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ := transform.String(t, s)
	return result
}

func stripLeadingArticle(s string) string {
	s = strings.TrimSpace(s)
	articles := []string{"the ", "a ", "an "}
	for _, art := range articles {
		if strings.HasPrefix(s, art) {
			return strings.TrimPrefix(s, art)
		}
	}
	return s
}
