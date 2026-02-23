//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"go-file-explorer/internal/storage"
)

func TestOperationsRenameMoveCopyDelete(t *testing.T) {
	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	seedA, err := store.OpenForWrite("/docs/alpha.txt")
	require.NoError(t, err)
	_, err = seedA.WriteString("alpha")
	require.NoError(t, err)
	require.NoError(t, seedA.Close())

	seedB, err := store.OpenForWrite("/docs/beta.txt")
	require.NoError(t, err)
	_, err = seedB.WriteString("beta")
	require.NoError(t, err)
	require.NoError(t, seedB.Close())

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	renamePayload := map[string]any{"path": "/docs/alpha.txt", "new_name": "alpha-renamed.txt"}
	renameBody, err := json.Marshal(renamePayload)
	require.NoError(t, err)
	renameResp := doAuthJSONRequest(t, http.MethodPut, server.URL+"/api/v1/files/rename", renameBody, accessToken)
	t.Cleanup(func() { _ = renameResp.Body.Close() })
	require.Equal(t, http.StatusOK, renameResp.StatusCode)

	movePayload := map[string]any{"sources": []string{"/docs/alpha-renamed.txt"}, "destination": "/archive"}
	moveBody, err := json.Marshal(movePayload)
	require.NoError(t, err)
	moveResp := doAuthJSONRequest(t, http.MethodPut, server.URL+"/api/v1/files/move", moveBody, accessToken)
	t.Cleanup(func() { _ = moveResp.Body.Close() })
	require.Equal(t, http.StatusOK, moveResp.StatusCode)

	copyPayload := map[string]any{"sources": []string{"/archive/alpha-renamed.txt"}, "destination": "/backup"}
	copyBody, err := json.Marshal(copyPayload)
	require.NoError(t, err)
	copyResp := doAuthJSONRequest(t, http.MethodPost, server.URL+"/api/v1/files/copy", copyBody, accessToken)
	t.Cleanup(func() { _ = copyResp.Body.Close() })
	require.Equal(t, http.StatusOK, copyResp.StatusCode)

	deletePayload := map[string]any{"paths": []string{"/backup/alpha-renamed.txt", "/docs/beta.txt"}}
	deleteBody, err := json.Marshal(deletePayload)
	require.NoError(t, err)
	deleteResp := doAuthJSONRequest(t, http.MethodDelete, server.URL+"/api/v1/files", deleteBody, accessToken)
	t.Cleanup(func() { _ = deleteResp.Body.Close() })
	require.Equal(t, http.StatusOK, deleteResp.StatusCode)

	if _, err := store.Stat("/archive/alpha-renamed.txt"); err != nil {
		t.Fatalf("expected moved file to exist: %v", err)
	}
	if _, err := store.Stat("/backup/alpha-renamed.txt"); err == nil {
		t.Fatalf("expected copied file to be deleted")
	}
	if _, err := store.Stat("/docs/beta.txt"); err == nil {
		t.Fatalf("expected deleted file to be removed")
	}
}

func TestOperationsSoftDeleteAndRestore(t *testing.T) {
	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	seed, err := store.OpenForWrite("/docs/to-restore.txt")
	require.NoError(t, err)
	_, err = seed.WriteString("restore me")
	require.NoError(t, err)
	require.NoError(t, seed.Close())

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	deletePayload := map[string]any{"paths": []string{"/docs/to-restore.txt"}}
	deleteBody, err := json.Marshal(deletePayload)
	require.NoError(t, err)
	deleteResp := doAuthJSONRequest(t, http.MethodDelete, server.URL+"/api/v1/files", deleteBody, accessToken)
	t.Cleanup(func() { _ = deleteResp.Body.Close() })
	require.Equal(t, http.StatusOK, deleteResp.StatusCode)

	if _, err := store.Stat("/docs/to-restore.txt"); err == nil {
		t.Fatalf("expected file to be moved to trash")
	}

	restorePayload := map[string]any{"paths": []string{"/docs/to-restore.txt"}}
	restoreBody, err := json.Marshal(restorePayload)
	require.NoError(t, err)
	restoreResp := doAuthJSONRequest(t, http.MethodPost, server.URL+"/api/v1/files/restore", restoreBody, accessToken)
	t.Cleanup(func() { _ = restoreResp.Body.Close() })
	require.Equal(t, http.StatusOK, restoreResp.StatusCode)

	if _, err := store.Stat("/docs/to-restore.txt"); err != nil {
		t.Fatalf("expected file to be restored: %v", err)
	}
}
