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
		if unicode.IsControl(char) {
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

	if len(cleaned) > 255 {
		cleaned = cleaned[:255]
	}

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
