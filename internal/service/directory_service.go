package service

import (
	"context"
	"fmt"
	"net/http"
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

type DirectoryService struct {
	store *storage.Storage
}

func NewDirectoryService(store *storage.Storage) *DirectoryService {
	return &DirectoryService{store: store}
}

func (s *DirectoryService) List(_ context.Context, requestedPath string, page int, limit int, sortBy string, order string) (model.DirectoryListData, model.Meta, error) {
	if page < 1 {
		page = 1
	}

	if limit <= 0 {
		limit = 50
	}

	if limit > 200 {
		limit = 200
	}

	resolved, err := s.store.Resolve(requestedPath)
	if err != nil {
		return model.DirectoryListData{}, model.Meta{}, err
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return model.DirectoryListData{}, model.Meta{}, apierror.New("NOT_FOUND", "directory not found", requestedPath, http.StatusNotFound)
		}
		return model.DirectoryListData{}, model.Meta{}, err
	}

	items := make([]model.FileItem, 0, len(entries))
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}

		apiPath := toAPIPath(filepath.Join(resolved, entry.Name()), s.store.RootAbs())
		item := model.FileItem{
			Name:        entry.Name(),
			Path:        apiPath,
			Permissions: info.Mode().String(),
			ModifiedAt:  info.ModTime().UTC(),
			CreatedAt:   info.ModTime().UTC(),
		}

		if entry.IsDir() {
			item.Type = "directory"
			item.Size = 0
			children, childrenErr := os.ReadDir(filepath.Join(resolved, entry.Name()))
			if childrenErr == nil {
				count := len(children)
				item.ItemCount = &count
			}
		} else {
			item.Type = "file"
			item.Size = info.Size()
			item.SizeHuman = humanizeSize(info.Size())
			item.Extension = strings.ToLower(filepath.Ext(entry.Name()))
		}

		items = append(items, item)
	}

	sortItems(items, sortBy, order)

	total := len(items)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	currentPath := requestedPath
	if strings.TrimSpace(currentPath) == "" {
		currentPath = "/"
	}

	currentPath = normalizeAPIPath(currentPath)

	parentPath := "/"
	if currentPath != "/" {
		parentPath = normalizeAPIPath(filepath.Dir(currentPath))
	}

	data := model.DirectoryListData{
		CurrentPath: currentPath,
		ParentPath:  parentPath,
		Items:       items[start:end],
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	meta := model.Meta{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}

	return data, meta, nil
}

func (s *DirectoryService) Create(_ context.Context, basePath string, name string) (model.DirectoryCreateData, error) {
	safeName, err := util.SanitizeFilename(name, false)
	if err != nil {
		return model.DirectoryCreateData{}, err
	}

	if strings.TrimSpace(basePath) == "" {
		basePath = "/"
	}

	fullPath := normalizeAPIPath(filepath.Join(basePath, safeName))
	resolved, err := s.store.Resolve(fullPath)
	if err != nil {
		return model.DirectoryCreateData{}, err
	}

	if _, statErr := os.Stat(resolved); statErr == nil {
		return model.DirectoryCreateData{}, apierror.New("ALREADY_EXISTS", "directory already exists", fullPath, http.StatusConflict)
	}

	if mkErr := s.store.MkdirAll(fullPath, 0o755); mkErr != nil {
		return model.DirectoryCreateData{}, mkErr
	}

	data := model.DirectoryCreateData{
		Name:      safeName,
		Path:      fullPath,
		Type:      "directory",
		CreatedAt: time.Now().UTC(),
	}

	return data, nil
}

func sortItems(items []model.FileItem, sortBy string, order string) {
	field := strings.ToLower(strings.TrimSpace(sortBy))
	if field == "" {
		field = "name"
	}

	ascending := strings.ToLower(strings.TrimSpace(order)) != "desc"

	less := func(i int, j int) bool {
		switch field {
		case "size":
			return items[i].Size < items[j].Size
		case "modified_at":
			return items[i].ModifiedAt.Before(items[j].ModifiedAt)
		case "type":
			if items[i].Type == items[j].Type {
				return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
			}
			return items[i].Type < items[j].Type
		default:
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
	}

	sort.SliceStable(items, func(i int, j int) bool {
		if ascending {
			return less(i, j)
		}
		return !less(i, j)
	})
}

func humanizeSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}

	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(size)
	for _, unit := range units {
		value = value / 1024
		if value < 1024 {
			return fmt.Sprintf("%.0f %s", value, unit)
		}
	}

	return fmt.Sprintf("%.0f PB", value/1024)
}

func toAPIPath(absPath string, rootAbs string) string {
	rel, err := filepath.Rel(rootAbs, absPath)
	if err != nil {
		return "/"
	}

	rel = filepath.ToSlash(rel)
	if rel == "." || rel == "" {
		return "/"
	}

	return normalizeAPIPath("/" + rel)
}

func normalizeAPIPath(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if cleaned == "." || cleaned == "" {
		return "/"
	}

	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}

	return cleaned
}
