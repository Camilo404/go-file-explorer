//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"go-file-explorer/internal/storage"
)

func TestDirectoryCreateAndList(t *testing.T) {
	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	body := map[string]any{
		"path": "/",
		"name": "documents",
	}
	payload, err := json.Marshal(body)
	require.NoError(t, err)

	createResp := doAuthJSONRequest(t, http.MethodPost, server.URL+"/api/v1/directories", payload, accessToken)
	t.Cleanup(func() { _ = createResp.Body.Close() })
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	listResp := doAuthRequest(t, http.MethodGet, server.URL+"/api/v1/files?path=/&page=1&limit=50&sort=name&order=asc", accessToken)
	t.Cleanup(func() { _ = listResp.Body.Close() })
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var listBody struct {
		Success bool `json:"success"`
		Data    struct {
			CurrentPath string `json:"current_path"`
			ParentPath  string `json:"parent_path"`
			Items       []struct {
				Name string `json:"name"`
				Type string `json:"type"`
				Path string `json:"path"`
			} `json:"items"`
		} `json:"data"`
		Meta struct {
			Page  int `json:"page"`
			Limit int `json:"limit"`
			Total int `json:"total"`
		} `json:"meta"`
	}

	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&listBody))
	require.True(t, listBody.Success)
	require.Equal(t, "/", listBody.Data.CurrentPath)
	require.Equal(t, 1, listBody.Meta.Total)
	require.Len(t, listBody.Data.Items, 1)
	require.Equal(t, "documents", listBody.Data.Items[0].Name)
	require.Equal(t, "directory", listBody.Data.Items[0].Type)
	require.Equal(t, "/documents", listBody.Data.Items[0].Path)
}
