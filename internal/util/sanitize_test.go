package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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
		require.Len(t, actual, 255)
	})
}
