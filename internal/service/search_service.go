package service

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/storage"
	"go-file-explorer/internal/util"
	"go-file-explorer/pkg/apierror"
)

type SearchService struct {
	store      storage.Storage
	maxDepth   int
	timeout    time.Duration
	maxResults int
}

func NewSearchService(store storage.Storage, maxDepth int, timeout time.Duration) *SearchService {
	if maxDepth <= 0 {
		maxDepth = 10
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &SearchService{store: store, maxDepth: maxDepth, timeout: timeout, maxResults: 1000}
}

func (s *SearchService) Search(ctx context.Context, query string, startPath string, itemType string, extension string, page int, limit int) (map[string]any, model.Meta, error) {
	query = strings.TrimSpace(query)
	normalizedType := strings.ToLower(strings.TrimSpace(itemType))
	normalizedExt := strings.ToLower(strings.TrimSpace(extension))
	if normalizedExt != "" && !strings.HasPrefix(normalizedExt, ".") {
		normalizedExt = "." + normalizedExt
	}

	if query == "" && normalizedType == "" && normalizedExt == "" {
		return nil, model.Meta{}, apierror.New("BAD_REQUEST", "at least one filter is required: q, type, or ext", "q", http.StatusBadRequest)
	}

	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	if strings.TrimSpace(startPath) == "" {
		startPath = "/"
	}

	if isInternalStoragePath(startPath) {
		return nil, model.Meta{}, apierror.New("NOT_FOUND", "start path not found", startPath, http.StatusNotFound)
	}

	resolvedStart, err := s.store.Resolve(startPath)
	if err != nil {
		return nil, model.Meta{}, err
	}

	if _, err := os.Stat(resolvedStart); err != nil {
		if os.IsNotExist(err) {
			return nil, model.Meta{}, apierror.New("NOT_FOUND", "start path not found", startPath, http.StatusNotFound)
		}
		return nil, model.Meta{}, err
	}

	searchCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	queryLower := strings.ToLower(query)
	items := make([]model.FileItem, 0)
	depthRoot := resolvedStart

	walkErr := filepath.WalkDir(resolvedStart, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}

		select {
		case <-searchCtx.Done():
			return searchCtx.Err()
		default:
		}

		if path == resolvedStart {
			return nil
		}

		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if isInternalStorageEntry(entry.Name()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rel, err := filepath.Rel(depthRoot, path)
		if err != nil {
			return nil
		}
		depth := strings.Count(filepath.ToSlash(rel), "/") + 1
		if depth > s.maxDepth {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if queryLower != "" {
			nameLower := strings.ToLower(entry.Name())
			if !strings.Contains(nameLower, queryLower) {
				return nil
			}
		}

		isDir := entry.IsDir()
		if normalizedType == "file" && isDir {
			return nil
		}
		if normalizedType == "dir" && !isDir {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if normalizedExt != "" && normalizedExt != ext {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return nil
		}

		relToRoot, relErr := filepath.Rel(s.store.RootAbs(), path)
		if relErr != nil {
			return nil
		}
		apiPath := normalizeAPIPath(filepath.ToSlash(relToRoot))
		item := model.FileItem{
			Name:         entry.Name(),
			Path:         apiPath,
			Type:         map[bool]string{true: "directory", false: "file"}[isDir],
			Size:         info.Size(),
			Extension:    ext,
			ModifiedAt:   info.ModTime().UTC(),
			CreatedAt:    info.ModTime().UTC(),
			Permissions:  info.Mode().String(),
			MatchContext: entry.Name(),
		}
		if isDir {
			item.Size = 0
			item.Extension = ""
		} else if util.IsImageExtension(ext) {
			item.IsImage = true
			item.PreviewURL = "/api/v1/files/preview?path=" + url.QueryEscape(apiPath)
			if util.IsThumbnailExtension(ext) {
				item.ThumbnailURL = "/api/v1/files/thumbnail?path=" + url.QueryEscape(apiPath) + "&size=256"
			}
		} else if util.IsVideoExtension(ext) {
			item.IsVideo = true
			item.PreviewURL = "/api/v1/files/preview?path=" + url.QueryEscape(apiPath)
		}

		items = append(items, item)
		if len(items) >= s.maxResults {
			return errors.New("max results reached")
		}

		return nil
	})

	if walkErr != nil && !errors.Is(walkErr, context.DeadlineExceeded) && !errors.Is(walkErr, context.Canceled) && walkErr.Error() != "max results reached" {
		return nil, model.Meta{}, walkErr
	}

	sort.Slice(items, func(i int, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	total := len(items)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	meta := model.Meta{Page: page, Limit: limit, Total: total, TotalPages: totalPages}
	data := map[string]any{
		"query": query,
		"items": items[start:end],
	}

	return data, meta, nil
}
