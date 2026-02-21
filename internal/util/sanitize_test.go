package util

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func isValidUTF8(s string) bool {
	return utf8.ValidString(s)
}

func TestSanitizeFilename(t *testing.T) {
	t.Parallel()

	t.Run("sanitizes invalid characters", func(t *testing.T) {
		actual, err := SanitizeFilename(` report<2026>?.pdf `, false)
		require.NoError(t, err)
		require.Equal(t, "report_2026__.pdf", actual)
	})

	t.Run("rejects empty filenames", func(t *testing.T) {
		_, err := SanitizeFilename("   ", false)
		require.Error(t, err)
	})

	t.Run("rejects hidden filenames when disabled", func(t *testing.T) {
		_, err := SanitizeFilename(".env", false)
		require.Error(t, err)
	})

	t.Run("allows hidden filenames when enabled", func(t *testing.T) {
		actual, err := SanitizeFilename(".env", true)
		require.NoError(t, err)
		require.Equal(t, ".env", actual)
	})

	t.Run("rejects windows reserved names", func(t *testing.T) {
		_, err := SanitizeFilename("CON.txt", false)
		require.Error(t, err)
	})

	t.Run("truncates long filenames", func(t *testing.T) {
		tooLong := make([]byte, 300)
		for i := 0; i < len(tooLong); i++ {
			tooLong[i] = 'a'
		}

		actual, err := SanitizeFilename(string(tooLong), false)
		require.NoError(t, err)
		require.Len(t, []rune(actual), 255)
	})

	t.Run("strips zero-width characters", func(t *testing.T) {
		input := "Call\u200B of\u200B Duty\u200B screenshot.png"
		actual, err := SanitizeFilename(input, false)
		require.NoError(t, err)
		require.Equal(t, "Call of Duty screenshot.png", actual)
	})

	t.Run("strips all invisible unicode characters", func(t *testing.T) {
		input := "file\u200B\u200C\u200D\u2060\uFEFFname.txt"
		actual, err := SanitizeFilename(input, false)
		require.NoError(t, err)
		require.Equal(t, "filename.txt", actual)
	})

	t.Run("rejects filenames that become empty after stripping invisible chars", func(t *testing.T) {
		input := "\u200B\u200C\u200D"
		_, err := SanitizeFilename(input, false)
		require.Error(t, err)
	})

	t.Run("rune-safe truncation preserves multi-byte characters", func(t *testing.T) {
		// Build a filename with 260 multi-byte runes (é = 2 bytes each)
		runes := make([]rune, 260)
		for i := range runes {
			runes[i] = 'é'
		}
		input := string(runes) + ".txt"

		actual, err := SanitizeFilename(input, false)
		require.NoError(t, err)
		// Should be truncated to 255 runes, all valid UTF-8
		require.LessOrEqual(t, len([]rune(actual)), 255)
		require.True(t, isValidUTF8(actual), "result should be valid UTF-8")
	})
}
