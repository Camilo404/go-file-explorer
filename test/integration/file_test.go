//go:build integration

package integration

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"go-file-explorer/internal/storage"
)

func TestFileUploadDownloadAndPreview(t *testing.T) {
	t.Parallel()

	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	require.NoError(t, writer.WriteField("path", "/uploads"))

	filePart, err := writer.CreateFormFile("files", "hello.txt")
	require.NoError(t, err)
	_, err = filePart.Write([]byte("hello from integration test"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/files/upload", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+accessToken)

	uploadResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = uploadResp.Body.Close() })
	require.Equal(t, http.StatusOK, uploadResp.StatusCode)

	var uploadBody struct {
		Success bool `json:"success"`
		Data    struct {
			Uploaded []struct {
				Name string `json:"name"`
				Path string `json:"path"`
			} `json:"uploaded"`
			Failed []any `json:"failed"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(uploadResp.Body).Decode(&uploadBody))
	require.True(t, uploadBody.Success)
	require.Len(t, uploadBody.Data.Uploaded, 1)
	require.Len(t, uploadBody.Data.Failed, 0)
	require.Equal(t, "/uploads/hello.txt", uploadBody.Data.Uploaded[0].Path)

	downloadReq, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/files/download?path=/uploads/hello.txt", nil)
	require.NoError(t, err)
	downloadReq.Header.Set("Authorization", "Bearer "+accessToken)
	downloadResp, err := http.DefaultClient.Do(downloadReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = downloadResp.Body.Close() })
	require.Equal(t, http.StatusOK, downloadResp.StatusCode)
	require.Contains(t, downloadResp.Header.Get("Content-Disposition"), "attachment")

	downloaded, err := io.ReadAll(downloadResp.Body)
	require.NoError(t, err)
	require.Equal(t, "hello from integration test", string(downloaded))

	previewReq, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/files/preview?path=/uploads/hello.txt", nil)
	require.NoError(t, err)
	previewReq.Header.Set("Authorization", "Bearer "+accessToken)
	previewResp, err := http.DefaultClient.Do(previewReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = previewResp.Body.Close() })
	require.Equal(t, http.StatusOK, previewResp.StatusCode)
	require.Contains(t, previewResp.Header.Get("Content-Disposition"), "inline")

	previewed, err := io.ReadAll(previewResp.Body)
	require.NoError(t, err)
	require.Equal(t, "hello from integration test", string(previewed))
}

func TestDirectoryDownloadAsZip(t *testing.T) {
	t.Parallel()

	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	writer, err := store.OpenForWrite("/docs/report.txt")
	require.NoError(t, err)
	_, err = writer.WriteString("report content")
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/files/download?path=/docs&archive=true", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/zip", resp.Header.Get("Content-Type"))

	zipBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	require.NoError(t, err)

	require.Len(t, zipReader.File, 1)
	require.Equal(t, "report.txt", zipReader.File[0].Name)
}

func TestFileInfoEndpoint(t *testing.T) {
	t.Parallel()

	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	writer, err := store.OpenForWrite("/docs/info.txt")
	require.NoError(t, err)
	_, err = writer.WriteString("file info")
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/files/info?path=/docs/info.txt", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Name      string `json:"name"`
			Path      string `json:"path"`
			Type      string `json:"type"`
			Extension string `json:"extension"`
			MimeType  string `json:"mime_type"`
		} `json:"data"`
	}

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.True(t, payload.Success)
	require.Equal(t, "info.txt", payload.Data.Name)
	require.Equal(t, "/docs/info.txt", payload.Data.Path)
	require.Equal(t, "file", payload.Data.Type)
	require.Equal(t, ".txt", payload.Data.Extension)
	require.NotEmpty(t, payload.Data.MimeType)
}
