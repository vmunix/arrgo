package release

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// romanNumeralRegex matches Roman numerals II-IX when preceded by a space (not at start of string).
// Does NOT match standalone "I" to avoid false positives like "I Robot".
// Does NOT match standalone "X" to avoid false positives like "SPY x FAMILY", "American History X".
// Does NOT match at start of string to avoid false positives like "VII Days".
// Case-insensitive to work with lowercased input from CleanTitle.
var romanNumeralRegex = regexp.MustCompile(`(?i) (ii|iii|iv|v|vi|vii|viii|ix)\b`)

var romanToArabic = map[string]string{
	"II": "2", "III": "3", "IV": "4", "V": "5",
	"VI": "6", "VII": "7", "VIII": "8", "IX": "9",
}

// NormalizeRomanNumerals converts Roman numerals (II-IX) to Arabic numbers.
// Does not convert standalone "I" to avoid false positives.
// Does not convert Roman numerals at the start of the string.
func NormalizeRomanNumerals(s string) string {
	return romanNumeralRegex.ReplaceAllStringFunc(s, func(match string) string {
		// match includes leading space, extract the Roman numeral part
		roman := strings.TrimSpace(match)
		if arabic, ok := romanToArabic[strings.ToUpper(roman)]; ok {
			return " " + arabic
		}
		return match
	})
}

// CleanTitle normalizes a title for matching purposes.
// Removes articles, punctuation, accents, normalizes whitespace, and converts Roman numerals.
func CleanTitle(title string) string {
	s := strings.ToLower(title)

	// Convert Roman numerals to Arabic numbers (must be before accent removal)
	s = NormalizeRomanNumerals(s)

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

// NormalizeSearchQuery prepares a search query for indexer APIs.
// Converts & to "and" and collapses whitespace.
// Unlike CleanTitle, preserves case and most punctuation for better search results.
func NormalizeSearchQuery(query string) string {
	s := strings.ReplaceAll(query, "&", "and")
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
