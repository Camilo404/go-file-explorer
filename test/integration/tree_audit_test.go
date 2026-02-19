//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"go-file-explorer/internal/storage"
)

func TestTreeEndpointLazyLoad(t *testing.T) {
	t.Parallel()

	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	createDirectory := func(path string, name string) {
		payload, marshalErr := json.Marshal(map[string]string{"path": path, "name": name})
		require.NoError(t, marshalErr)
		resp := doAuthJSONRequest(t, http.MethodPost, server.URL+"/api/v1/directories", payload, accessToken)
		t.Cleanup(func() { _ = resp.Body.Close() })
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	createDirectory("/", "docs")
	createDirectory("/docs", "invoices")

	resp := doAuthRequest(t, http.MethodGet, server.URL+"/api/v1/tree?path=/docs&depth=1&include_files=false&page=1&limit=20", accessToken)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Path  string `json:"path"`
			Nodes []struct {
				Name        string `json:"name"`
				Type        string `json:"type"`
				HasChildren bool   `json:"has_children"`
			} `json:"nodes"`
		} `json:"data"`
		Meta struct {
			Page  int `json:"page"`
			Limit int `json:"limit"`
			Total int `json:"total"`
		} `json:"meta"`
	}

	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.True(t, body.Success)
	require.Equal(t, "/docs", body.Data.Path)
	require.Equal(t, 1, body.Meta.Total)
	require.Len(t, body.Data.Nodes, 1)
	require.Equal(t, "invoices", body.Data.Nodes[0].Name)
	require.Equal(t, "directory", body.Data.Nodes[0].Type)
	require.False(t, body.Data.Nodes[0].HasChildren)
}

func TestAuditEndpointAdminOnlyAndFilters(t *testing.T) {
	t.Parallel()

	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	server, accessToken, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	createDirectoryPayload, err := json.Marshal(map[string]string{"path": "/", "name": "audit-dir"})
	require.NoError(t, err)
	createResp := doAuthJSONRequest(t, http.MethodPost, server.URL+"/api/v1/directories", createDirectoryPayload, accessToken)
	t.Cleanup(func() { _ = createResp.Body.Close() })
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	renamePayload, err := json.Marshal(map[string]string{"path": "/audit-dir", "new_name": "audit-dir-renamed"})
	require.NoError(t, err)
	renameResp := doAuthJSONRequest(t, http.MethodPut, server.URL+"/api/v1/files/rename", renamePayload, accessToken)
	t.Cleanup(func() { _ = renameResp.Body.Close() })
	require.Equal(t, http.StatusOK, renameResp.StatusCode)

	auditResp := doAuthRequest(t, http.MethodGet, server.URL+"/api/v1/audit?action=rename&status=success&page=1&limit=20", accessToken)
	t.Cleanup(func() { _ = auditResp.Body.Close() })
	require.Equal(t, http.StatusOK, auditResp.StatusCode)

	var auditBody struct {
		Success bool `json:"success"`
		Data    struct {
			Items []struct {
				Action string `json:"action"`
				Status string `json:"status"`
			} `json:"items"`
		} `json:"data"`
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}

	require.NoError(t, json.NewDecoder(auditResp.Body).Decode(&auditBody))
	require.True(t, auditBody.Success)
	require.GreaterOrEqual(t, auditBody.Meta.Total, 1)
	require.NotEmpty(t, auditBody.Data.Items)
	require.Equal(t, "rename", auditBody.Data.Items[0].Action)
	require.Equal(t, "success", auditBody.Data.Items[0].Status)
}
