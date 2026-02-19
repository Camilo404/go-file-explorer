package service

import (
	"context"
	"fmt"
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
			if util.IsImageExtension(item.Extension) {
				item.IsImage = true
				item.PreviewURL = "/api/v1/files/preview?path=" + url.QueryEscape(apiPath)
				if util.IsThumbnailExtension(item.Extension) {
					item.ThumbnailURL = "/api/v1/files/thumbnail?path=" + url.QueryEscape(apiPath) + "&size=256"
				}
			}
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

func (s *DirectoryService) Tree(_ context.Context, requestedPath string, depth int, includeFiles bool, page int, limit int) (model.TreeData, model.Meta, error) {
	if depth <= 0 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}

	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}

	resolved, err := s.store.Resolve(requestedPath)
	if err != nil {
		return model.TreeData{}, model.Meta{}, err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return model.TreeData{}, model.Meta{}, apierror.New("NOT_FOUND", "directory not found", requestedPath, http.StatusNotFound)
		}
		return model.TreeData{}, model.Meta{}, err
	}
	if !info.IsDir() {
		return model.TreeData{}, model.Meta{}, apierror.New("BAD_REQUEST", "path points to a file", requestedPath, http.StatusBadRequest)
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return model.TreeData{}, model.Meta{}, err
	}

	candidates := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if !includeFiles && !entry.IsDir() {
			continue
		}
		candidates = append(candidates, entry)
	}

	sort.SliceStable(candidates, func(i int, j int) bool {
		return strings.ToLower(candidates[i].Name()) < strings.ToLower(candidates[j].Name())
	})

	total := len(candidates)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	basePath := normalizeAPIPath(requestedPath)
	nodes := make([]model.TreeNode, 0, end-start)
	for _, entry := range candidates[start:end] {
		node, nodeErr := s.buildTreeNode(filepath.Join(resolved, entry.Name()), normalizeAPIPath(filepath.Join(basePath, entry.Name())), entry, depth-1, includeFiles)
		if nodeErr != nil {
			continue
		}
		nodes = append(nodes, node)
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	meta := model.Meta{Page: page, Limit: limit, Total: total, TotalPages: totalPages}
	data := model.TreeData{Path: basePath, Nodes: nodes}
	return data, meta, nil
}

func (s *DirectoryService) buildTreeNode(absPath string, apiPath string, entry os.DirEntry, remainingDepth int, includeFiles bool) (model.TreeNode, error) {
	info, err := entry.Info()
	if err != nil {
		return model.TreeNode{}, err
	}

	node := model.TreeNode{
		Name:       entry.Name(),
		Path:       apiPath,
		Type:       map[bool]string{true: "directory", false: "file"}[entry.IsDir()],
		ModifiedAt: info.ModTime().UTC(),
	}

	if !entry.IsDir() {
		node.HasChildren = false
		return node, nil
	}

	children, err := os.ReadDir(absPath)
	if err != nil {
		return node, nil
	}

	visibleChildren := make([]os.DirEntry, 0, len(children))
	for _, child := range children {
		if child.Type()&os.ModeSymlink != 0 {
			continue
		}
		if !includeFiles && !child.IsDir() {
			continue
		}
		visibleChildren = append(visibleChildren, child)
	}

	count := len(visibleChildren)
	node.ItemCount = &count
	node.HasChildren = count > 0

	if remainingDepth <= 0 || count == 0 {
		return node, nil
	}

	sort.SliceStable(visibleChildren, func(i int, j int) bool {
		return strings.ToLower(visibleChildren[i].Name()) < strings.ToLower(visibleChildren[j].Name())
	})

	node.Children = make([]model.TreeNode, 0, len(visibleChildren))
	for _, child := range visibleChildren {
		childNode, childErr := s.buildTreeNode(filepath.Join(absPath, child.Name()), normalizeAPIPath(filepath.Join(apiPath, child.Name())), child, remainingDepth-1, includeFiles)
		if childErr != nil {
			continue
		}
		node.Children = append(node.Children, childNode)
	}

	return node, nil
}

func sortItems(items []model.FileItem, sortBy string, order string) {
	field := strings.ToLower(strings.TrimSpace(sortBy))
	if field == "" {
		field = "name"
	}

	ascending := strings.ToLower(strings.TrimSpace(order)) != "desc"

	compare := func(i int, j int) int {
		switch field {
		case "size":
			if items[i].Size < items[j].Size {
				return -1
			}
			if items[i].Size > items[j].Size {
				return 1
			}
			return 0
		case "modified_at":
			if items[i].ModifiedAt.Before(items[j].ModifiedAt) {
				return -1
			}
			if items[i].ModifiedAt.After(items[j].ModifiedAt) {
				return 1
			}
			return 0
		case "type":
			if items[i].Type == items[j].Type {
				left := strings.ToLower(items[i].Name)
				right := strings.ToLower(items[j].Name)
				if left < right {
					return -1
				}
				if left > right {
					return 1
				}
				return 0
			}
			if items[i].Type < items[j].Type {
				return -1
			}
			if items[i].Type > items[j].Type {
				return 1
			}
			return 0
		default:
			left := strings.ToLower(items[i].Name)
			right := strings.ToLower(items[j].Name)
			if left < right {
				return -1
			}
			if left > right {
				return 1
			}
			return 0
		}
	}

	sort.SliceStable(items, func(i int, j int) bool {
		cmp := compare(i, j)
		if cmp == 0 {
			return false
		}
		if ascending {
			return cmp < 0
		}
		return cmp > 0
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
