package release

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// romanNumeralRegex matches Roman numerals II-X when preceded by a space (not at start of string).
// Does NOT match standalone "I" to avoid false positives like "I Robot".
// Does NOT match at start of string to avoid false positives like "VII Days".
var romanNumeralRegex = regexp.MustCompile(` (II|III|IV|V|VI|VII|VIII|IX|X)\b`)

var romanToArabic = map[string]string{
	"II": "2", "III": "3", "IV": "4", "V": "5",
	"VI": "6", "VII": "7", "VIII": "8", "IX": "9", "X": "10",
}

// NormalizeRomanNumerals converts Roman numerals (II-X) to Arabic numbers.
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
