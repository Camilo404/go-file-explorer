package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/storage"
)

type StorageHandler struct {
	store *storage.Storage
}

func NewStorageHandler(store *storage.Storage) *StorageHandler {
	return &StorageHandler{store: store}
}

func (h *StorageHandler) Stats(w http.ResponseWriter, r *http.Request) {
	root := h.store.RootAbs()

	var totalSize int64
	var fileCount int
	var directoryCount int

	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
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
