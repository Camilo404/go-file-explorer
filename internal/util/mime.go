package util

import (
	"net/http"
	"os"
	"strings"
)

func DetectMIMEFromFile(file *os.File) (string, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil {
		return "", err
	}

	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}

	return http.DetectContentType(buffer[:n]), nil
}

func IsImageMIME(mimeType string) bool {
	cleaned := strings.ToLower(strings.TrimSpace(mimeType))
	return strings.HasPrefix(cleaned, "image/")
}

func IsImageExtension(extension string) bool {
	switch strings.ToLower(strings.TrimSpace(extension)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tiff", ".tif", ".svg", ".ico", ".avif", ".heic", ".heif":
		return true
	default:
		return false
	}
}

func IsThumbnailMIME(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp", "image/tiff":
		return true
	default:
		return false
	}
}

func IsThumbnailExtension(extension string) bool {
	switch strings.ToLower(strings.TrimSpace(extension)) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".tiff", ".tif":
		return true
	default:
		return false
	}
}
