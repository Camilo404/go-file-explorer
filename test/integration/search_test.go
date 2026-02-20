//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"go-file-explorer/internal/storage"
)

func TestSearchEndpointWithFilters(t *testing.T) {
	t.Parallel()

	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	reportA, err := store.OpenForWrite("/documents/finance/annual-report.pdf")
	require.NoError(t, err)
	_, err = reportA.WriteString("pdf content")
	require.NoError(t, err)
	require.NoError(t, reportA.Close())

	reportB, err := store.OpenForWrite("/documents/finance/annual-report.txt")
	require.NoError(t, err)
	_, err = reportB.WriteString("txt content")
	require.NoError(t, err)
	require.NoError(t, reportB.Close())

	if err := store.MkdirAll("/documents/reports", 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	resp := doAuthRequest(t, http.MethodGet, server.URL+"/api/v1/search?q=report&path=/documents&type=file&ext=.pdf&page=1&limit=20", accessToken)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Query string `json:"query"`
			Items []struct {
				Name string `json:"name"`
				Path string `json:"path"`
				Type string `json:"type"`
			} `json:"items"`
		} `json:"data"`
		Meta struct {
			Page       int `json:"page"`
			Limit      int `json:"limit"`
			Total      int `json:"total"`
			TotalPages int `json:"total_pages"`
		} `json:"meta"`
	}

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.True(t, payload.Success)
	require.Equal(t, "report", payload.Data.Query)
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, "annual-report.pdf", payload.Data.Items[0].Name)
	require.Equal(t, "/documents/finance/annual-report.pdf", payload.Data.Items[0].Path)
	require.Equal(t, "file", payload.Data.Items[0].Type)
	require.Equal(t, 1, payload.Meta.Total)
}

func TestSearchEndpointWithExtensionOnly(t *testing.T) {
	t.Parallel()

	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	pdfFile, err := store.OpenForWrite("/documents/finance/annual-report.pdf")
	require.NoError(t, err)
	_, err = pdfFile.WriteString("pdf content")
	require.NoError(t, err)
	require.NoError(t, pdfFile.Close())

	txtFile, err := store.OpenForWrite("/documents/finance/annual-report.txt")
	require.NoError(t, err)
	_, err = txtFile.WriteString("txt content")
	require.NoError(t, err)
	require.NoError(t, txtFile.Close())

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	resp := doAuthRequest(t, http.MethodGet, server.URL+"/api/v1/search?path=/documents&type=file&ext=pdf&page=1&limit=20", accessToken)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Name string `json:"name"`
				Path string `json:"path"`
				Type string `json:"type"`
			} `json:"items"`
		} `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data.Items, 1)
	require.Equal(t, "annual-report.pdf", payload.Data.Items[0].Name)
	require.Equal(t, "/documents/finance/annual-report.pdf", payload.Data.Items[0].Path)
	require.Equal(t, "file", payload.Data.Items[0].Type)
	require.Equal(t, 1, payload.Meta.Total)
}

func TestSearchEndpointRequiresAtLeastOneFilter(t *testing.T) {
	t.Parallel()

	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	resp := doAuthRequest(t, http.MethodGet, server.URL+"/api/v1/search?path=/", accessToken)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
