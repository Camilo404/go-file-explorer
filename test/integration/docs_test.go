//go:build integration

package integration

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"go-file-explorer/internal/storage"
)

func TestOpenAPISpecAndSwaggerUI(t *testing.T) {
	t.Parallel()

	store, err := storage.New(t.TempDir())
	require.NoError(t, err)

	server, _, _ := newAuthedServer(t, store)
	t.Cleanup(server.Close)

	specResp, err := http.Get(server.URL + "/openapi.yaml")
	require.NoError(t, err)
	t.Cleanup(func() { _ = specResp.Body.Close() })
	require.Equal(t, http.StatusOK, specResp.StatusCode)
	require.Equal(t, "application/yaml", specResp.Header.Get("Content-Type"))

	specBytes, err := io.ReadAll(specResp.Body)
	require.NoError(t, err)
	specText := string(specBytes)
	require.Contains(t, specText, "openapi: 3.0.3")
	require.Contains(t, specText, "/api/v1/jobs/operations")
	require.Contains(t, specText, "/api/v1/tree")
	require.Contains(t, specText, "/api/v1/audit")

	swaggerResp, err := http.Get(server.URL + "/swagger")
	require.NoError(t, err)
	t.Cleanup(func() { _ = swaggerResp.Body.Close() })
	require.Equal(t, http.StatusOK, swaggerResp.StatusCode)
	require.Contains(t, swaggerResp.Header.Get("Content-Type"), "text/html")

	swaggerBytes, err := io.ReadAll(swaggerResp.Body)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(swaggerBytes), "SwaggerUIBundle"))
}
