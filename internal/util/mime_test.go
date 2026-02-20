package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsImageExtension(t *testing.T) {
	t.Parallel()

	require.True(t, IsImageExtension(".png"))
	require.True(t, IsImageExtension(".jfif"))
	require.True(t, IsImageExtension(".avif"))
	require.True(t, IsImageExtension(".jxl"))
	require.True(t, IsImageExtension(" .JPEG "))
	require.False(t, IsImageExtension(".pdf"))
	require.False(t, IsImageExtension(""))
}

func TestIsThumbnailExtension(t *testing.T) {
	t.Parallel()

	require.True(t, IsThumbnailExtension(".jpg"))
	require.True(t, IsThumbnailExtension(".jfif"))
	require.True(t, IsThumbnailExtension(".png"))
	require.True(t, IsThumbnailExtension(" .WEBP "))
	require.False(t, IsThumbnailExtension(".avif"))
	require.False(t, IsThumbnailExtension(".svg"))
	require.False(t, IsThumbnailExtension(".txt"))
}
