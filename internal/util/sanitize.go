package util

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"go-file-explorer/pkg/apierror"
)

var invalidFilenameChars = regexp.MustCompile(`[<>:"/\\|?*]`)

var windowsReservedNames = map[string]struct{}{
	"CON":  {},
	"PRN":  {},
	"AUX":  {},
	"NUL":  {},
	"COM1": {},
	"COM2": {},
	"COM3": {},
	"COM4": {},
	"COM5": {},
	"COM6": {},
	"COM7": {},
	"COM8": {},
	"COM9": {},
	"LPT1": {},
	"LPT2": {},
	"LPT3": {},
	"LPT4": {},
	"LPT5": {},
	"LPT6": {},
	"LPT7": {},
	"LPT8": {},
	"LPT9": {},
}

func SanitizeFilename(name string, allowHidden bool) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", apierror.New("INVALID_FILENAME", "filename cannot be empty", "", 400)
	}

	if strings.Contains(trimmed, "\x00") {
		return "", apierror.New("INVALID_FILENAME", "filename contains null bytes", trimmed, 400)
	}

	builder := strings.Builder{}
	builder.Grow(len(trimmed))

	for _, char := range trimmed {
		if unicode.IsControl(char) || isInvisibleUnicode(char) {
			continue
		}

		builder.WriteRune(char)
	}

	withoutControl := builder.String()
	replaced := invalidFilenameChars.ReplaceAllString(withoutControl, "_")
	cleaned := strings.TrimSpace(replaced)

	if cleaned == "" {
		return "", apierror.New("INVALID_FILENAME", "filename is invalid after sanitization", trimmed, 400)
	}

	// Truncate by runes (not bytes) to avoid splitting multi-byte characters.
	runes := []rune(cleaned)
	if len(runes) > 255 {
		runes = runes[:255]
	}
	cleaned = string(runes)

	if strings.HasPrefix(cleaned, ".") && !allowHidden {
		return "", apierror.New("INVALID_FILENAME", "hidden filenames are not allowed", cleaned, 400)
	}

	stem := cleaned
	if idx := strings.Index(cleaned, "."); idx >= 0 {
		stem = cleaned[:idx]
	}

	if _, exists := windowsReservedNames[strings.ToUpper(stem)]; exists {
		return "", apierror.New("INVALID_FILENAME", "reserved filename is not allowed", cleaned, 400)
	}

	if cleaned == "." || cleaned == ".." {
		return "", apierror.New("INVALID_FILENAME", "filename cannot be current or parent directory", cleaned, 400)
	}

	if strings.Contains(cleaned, string(rune(0))) {
		return "", fmt.Errorf("unexpected null byte in filename")
	}

	return cleaned, nil
}

// isInvisibleUnicode returns true for zero-width, formatting, and other
// invisible Unicode characters that should be stripped from filenames.
func isInvisibleUnicode(r rune) bool {
	switch r {
	case
		'\u200B', // Zero-Width Space
		'\u200C', // Zero-Width Non-Joiner
		'\u200D', // Zero-Width Joiner
		'\u200E', // Left-to-Right Mark
		'\u200F', // Right-to-Left Mark
		'\u2060', // Word Joiner
		'\u2061', // Function Application
		'\u2062', // Invisible Times
		'\u2063', // Invisible Separator
		'\u2064', // Invisible Plus
		'\uFEFF', // Zero-Width No-Break Space / BOM
		'\uFFF9', // Interlinear Annotation Anchor
		'\uFFFA', // Interlinear Annotation Separator
		'\uFFFB': // Interlinear Annotation Terminator
		return true
	}

	// Unicode categories for format and non-characters
	if unicode.Is(unicode.Cf, r) { // Format characters (Cf category)
		return true
	}

	return false
}
