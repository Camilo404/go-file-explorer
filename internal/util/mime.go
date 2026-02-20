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

func IsVideoMIME(mimeType string) bool {
	cleaned := strings.ToLower(strings.TrimSpace(mimeType))
	return strings.HasPrefix(cleaned, "video/")
}

func IsImageExtension(extension string) bool {
	switch strings.ToLower(strings.TrimSpace(extension)) {
	case ".png", ".apng", ".jpg", ".jpeg", ".jpe", ".jfif", ".pjpeg", ".pjp", ".gif", ".webp", ".bmp", ".dib", ".tiff", ".tif", ".svg", ".svgz", ".ico", ".cur", ".avif", ".heic", ".heif", ".jxl", ".jp2", ".j2k", ".jpf", ".jpm", ".mj2":
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
	case ".jpg", ".jpeg", ".jpe", ".jfif", ".pjpeg", ".pjp", ".png", ".gif", ".webp", ".bmp", ".dib", ".tiff", ".tif":
		return true
	default:
		return false
	}
}

func IsVideoExtension(extension string) bool {
	switch strings.ToLower(strings.TrimSpace(extension)) {
	case ".mp4", ".m4v", ".mov", ".webm", ".mkv", ".avi", ".wmv", ".flv", ".mpeg", ".mpg", ".m2v", ".3gp", ".3g2", ".ts", ".mts", ".m2ts", ".ogv", ".qt", ".asf":
		return true
	default:
		return false
	}
}
