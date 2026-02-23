package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/storage"
)

type StorageHandler struct {
	store        storage.Storage
	excludePaths []string // absolute paths to skip when walking
}

func NewStorageHandler(store storage.Storage, excludePaths []string) *StorageHandler {
	cleaned := make([]string, 0, len(excludePaths))
	for _, p := range excludePaths {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		cleaned = append(cleaned, abs)
	}
	return &StorageHandler{store: store, excludePaths: cleaned}
}

func (h *StorageHandler) Stats(w http.ResponseWriter, r *http.Request) {
	root := h.store.RootAbs()

	var totalSize int64
	var fileCount int
	var directoryCount int

	err := filepath.Walk(root, func(currentPath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		abs, absErr := filepath.Abs(currentPath)
		if absErr != nil {
			return nil
		}

		// Skip the root directory itself.
		if abs == root {
			return nil
		}

		// Skip excluded directories (trash, thumbnails, etc.) and their contents.
		if info.IsDir() {
			for _, excluded := range h.excludePaths {
				if strings.EqualFold(abs, excluded) || strings.HasPrefix(strings.ToLower(abs)+string(filepath.Separator), strings.ToLower(excluded)+string(filepath.Separator)) {
					return filepath.SkipDir
				}
			}
		}

		if info.IsDir() {
			directoryCount++
		} else {
			fileCount++
			totalSize += info.Size()
		}
		return nil
	})
	if err != nil {
		writeError(w, err)
		return
	}

	stats := model.StorageStats{
		TotalSize:      totalSize,
		TotalSizeHuman: humanizeBytes(totalSize),
		FileCount:      fileCount,
		DirectoryCount: directoryCount,
	}

	writeSuccess(w, http.StatusOK, stats, nil)
}

func humanizeBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}
