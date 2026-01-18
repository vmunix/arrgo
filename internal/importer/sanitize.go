// internal/importer/sanitize.go
package importer

import (
	"path/filepath"
	"regexp"
	"strings"
)

// illegalChars are characters not allowed in filenames on common filesystems.
var illegalChars = regexp.MustCompile(`[<>:"/\\|?*\x00]`)

// multiSpace matches multiple consecutive spaces.
var multiSpace = regexp.MustCompile(`\s+`)

// multiDot matches multiple consecutive dots.
var multiDot = regexp.MustCompile(`\.{2,}`)

// SanitizeFilename removes or replaces characters that are unsafe for filenames.
// This prevents path traversal attacks and filesystem errors.
func SanitizeFilename(name string) string {
	// Remove null bytes
	name = strings.ReplaceAll(name, "\x00", "")

	// Replace path separators with space
	name = strings.ReplaceAll(name, "/", " ")
	name = strings.ReplaceAll(name, "\\", " ")

	// Replace illegal characters with space
	name = illegalChars.ReplaceAllString(name, " ")

	// Collapse multiple dots to single dot
	name = multiDot.ReplaceAllString(name, ".")

	// Collapse multiple spaces to single space
	name = multiSpace.ReplaceAllString(name, " ")

	// Trim leading/trailing whitespace and dots
	name = strings.Trim(name, " .")

	return name
}

// ValidatePath ensures the path is within the expected root directory.
// Returns ErrPathTraversal if the path would escape the root.
func ValidatePath(path, expectedRoot string) error {
	// Clean both paths to resolve any . or .. components
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(expectedRoot)

	// Ensure root ends with separator for prefix check
	if !strings.HasSuffix(cleanRoot, string(filepath.Separator)) {
		cleanRoot += string(filepath.Separator)
	}

	// Path must start with root (or be exactly root without trailing slash)
	if cleanPath != filepath.Clean(expectedRoot) && !strings.HasPrefix(cleanPath, cleanRoot) {
		return ErrPathTraversal
	}

	return nil
}
