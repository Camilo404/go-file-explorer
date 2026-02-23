//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go-file-explorer/internal/storage"
)

func TestUploadConflictPolicyRename(t *testing.T) {
	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	seed, err := store.OpenForWrite("/uploads/file.txt")
	require.NoError(t, err)
	_, err = seed.WriteString("existing")
	require.NoError(t, err)
	require.NoError(t, seed.Close())

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("path", "/uploads"))
	filePart, err := writer.CreateFormFile("files", "file.txt")
	require.NoError(t, err)
	_, err = filePart.Write([]byte("new content"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/files/upload?conflict_policy=rename", body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Uploaded []struct {
				Path string `json:"path"`
			} `json:"uploaded"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Uploaded, 1)
	require.Equal(t, "/uploads/file (1).txt", payload.Data.Uploaded[0].Path)
}

func TestMoveConflictPolicySkipAndOverwrite(t *testing.T) {
	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	source, err := store.OpenForWrite("/source/a.txt")
	require.NoError(t, err)
	_, err = source.WriteString("source")
	require.NoError(t, err)
	require.NoError(t, source.Close())

	target, err := store.OpenForWrite("/target/a.txt")
	require.NoError(t, err)
	_, err = target.WriteString("target")
	require.NoError(t, err)
	require.NoError(t, target.Close())

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	skipBody, err := json.Marshal(map[string]any{
		"sources":         []string{"/source/a.txt"},
		"destination":     "/target",
		"conflict_policy": "skip",
	})
	require.NoError(t, err)
	skipResp := doAuthJSONRequest(t, http.MethodPut, server.URL+"/api/v1/files/move", skipBody, accessToken)
	t.Cleanup(func() { _ = skipResp.Body.Close() })
	require.Equal(t, http.StatusOK, skipResp.StatusCode)

	if _, statErr := store.Stat("/source/a.txt"); statErr != nil {
		t.Fatalf("expected source file to remain when skip policy is used: %v", statErr)
	}

	overwriteBody, err := json.Marshal(map[string]any{
		"sources":         []string{"/source/a.txt"},
		"destination":     "/target",
		"conflict_policy": "overwrite",
	})
	require.NoError(t, err)
	overwriteResp := doAuthJSONRequest(t, http.MethodPut, server.URL+"/api/v1/files/move", overwriteBody, accessToken)
	t.Cleanup(func() { _ = overwriteResp.Body.Close() })
	require.Equal(t, http.StatusOK, overwriteResp.StatusCode)

	if _, statErr := store.Stat("/source/a.txt"); statErr == nil {
		t.Fatalf("expected source file to be moved on overwrite")
	}

	movedFile, err := store.OpenForRead("/target/a.txt")
	require.NoError(t, err)
	defer movedFile.Close()
	bytes, err := io.ReadAll(movedFile)
	require.NoError(t, err)
	require.Equal(t, "source", string(bytes))
}

func TestAsyncJobOperationsLifecycle(t *testing.T) {
	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	f1, err := store.OpenForWrite("/batch/a.txt")
	require.NoError(t, err)
	_, err = f1.WriteString("a")
	require.NoError(t, err)
	require.NoError(t, f1.Close())

	f2, err := store.OpenForWrite("/batch/b.txt")
	require.NoError(t, err)
	_, err = f2.WriteString("b")
	require.NoError(t, err)
	require.NoError(t, f2.Close())

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	createBody, err := json.Marshal(map[string]any{
		"operation":       "copy",
		"sources":         []string{"/batch/a.txt", "/batch/b.txt"},
		"destination":     "/batch-copy",
		"conflict_policy": "rename",
	})
	require.NoError(t, err)

	createResp := doAuthJSONRequest(t, http.MethodPost, server.URL+"/api/v1/jobs/operations", createBody, accessToken)
	t.Cleanup(func() { _ = createResp.Body.Close() })
	require.Equal(t, http.StatusAccepted, createResp.StatusCode)

	var createPayload struct {
		Success bool `json:"success"`
		Data    struct {
			JobID string `json:"job_id"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(createResp.Body).Decode(&createPayload))
	require.True(t, createPayload.Success)
	require.NotEmpty(t, createPayload.Data.JobID)

	jobID := createPayload.Data.JobID
	status := ""
	for attempt := 0; attempt < 25; attempt++ {
		statusResp := doAuthRequest(t, http.MethodGet, server.URL+"/api/v1/jobs/"+jobID, accessToken)
		require.Equal(t, http.StatusOK, statusResp.StatusCode)

		var statusPayload struct {
			Success bool `json:"success"`
			Data    struct {
				Status string `json:"status"`
			} `json:"data"`
		}
		err = json.NewDecoder(statusResp.Body).Decode(&statusPayload)
		_ = statusResp.Body.Close()
		require.NoError(t, err)
		require.True(t, statusPayload.Success)

		status = statusPayload.Data.Status
		if status == "completed" || status == "partial" || status == "failed" {
			break
		}
		time.Sleep(80 * time.Millisecond)
	}

	require.Contains(t, []string{"completed", "partial", "failed"}, status)

	itemsResp := doAuthRequest(t, http.MethodGet, server.URL+"/api/v1/jobs/"+jobID+"/items?page=1&limit=50", accessToken)
	t.Cleanup(func() { _ = itemsResp.Body.Close() })
	require.Equal(t, http.StatusOK, itemsResp.StatusCode)

	var itemsPayload struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Status string `json:"status"`
			} `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(itemsResp.Body).Decode(&itemsPayload))
	require.True(t, itemsPayload.Success)
	require.GreaterOrEqual(t, len(itemsPayload.Data.Items), 2)

	for _, item := range itemsPayload.Data.Items {
		require.True(t, strings.TrimSpace(item.Status) != "")
	}
}
