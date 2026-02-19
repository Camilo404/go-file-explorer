package storage

import (
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStorageBasicOperations(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	store, err := New(root)
	require.NoError(t, err)

	require.NoError(t, store.MkdirAll("/docs", 0o755))

	writer, err := store.OpenForWrite("/docs/hello.txt")
	require.NoError(t, err)
	_, err = writer.WriteString("hello world")
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	info, err := store.Stat("/docs/hello.txt")
	require.NoError(t, err)
	require.False(t, info.IsDir())

	reader, err := store.OpenForRead("/docs/hello.txt")
	require.NoError(t, err)
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, "hello world", string(content))

	require.NoError(t, store.Rename("/docs/hello.txt", "/archive/renamed.txt"))
	_, err = store.Stat("/archive/renamed.txt")
	require.NoError(t, err)

	entries, err := store.ReadDir("/archive")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "renamed.txt", entries[0].Name())

	require.NoError(t, store.RemoveAll("/archive"))
	_, err = store.Stat("/archive")
	require.Error(t, err)
}
