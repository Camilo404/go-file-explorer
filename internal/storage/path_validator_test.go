package storage

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPathValidatorResolvePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	validator, err := NewPathValidator(root)
	require.NoError(t, err)

	t.Run("root path resolves to root", func(t *testing.T) {
		resolved, resolveErr := validator.ResolvePath("/")
		require.NoError(t, resolveErr)
		require.Equal(t, validator.RootAbs(), resolved)
	})

	t.Run("normal path resolves inside root", func(t *testing.T) {
		resolved, resolveErr := validator.ResolvePath("/documents/report.txt")
		require.NoError(t, resolveErr)
		require.Equal(t, filepath.Join(validator.RootAbs(), "documents", "report.txt"), resolved)
	})

	t.Run("backslashes are normalized", func(t *testing.T) {
		resolved, resolveErr := validator.ResolvePath(`documents\\photo.jpg`)
		require.NoError(t, resolveErr)
		require.Equal(t, filepath.Join(validator.RootAbs(), "documents", "photo.jpg"), resolved)
	})

	t.Run("path traversal is rejected", func(t *testing.T) {
		_, resolveErr := validator.ResolvePath("/documents/../secrets.txt")
		require.Error(t, resolveErr)
	})

	t.Run("control characters are rejected", func(t *testing.T) {
		_, resolveErr := validator.ResolvePath("documents\nreport.txt")
		require.Error(t, resolveErr)
	})

	t.Run("null bytes are rejected", func(t *testing.T) {
		_, resolveErr := validator.ResolvePath("documents\x00/report.txt")
		require.Error(t, resolveErr)
	})

	t.Run("within root check is platform-aware", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			require.True(t, isWithinRoot(`C:\Storage\Root`, `c:\storage\root\folder\file.txt`))
			return
		}

		require.False(t, isWithinRoot(`/tmp/Root`, `/tmp/root/folder/file.txt`))
	})
}
