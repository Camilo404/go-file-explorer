package service

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/storage"
	"go-file-explorer/internal/util"
	"go-file-explorer/pkg/apierror"
)

type OperationsService struct {
	store *storage.Storage
	trash *TrashService
	audit *AuditService
}

func NewOperationsService(store *storage.Storage, trash *TrashService, audit *AuditService) *OperationsService {
	return &OperationsService{store: store, trash: trash, audit: audit}
}

func (s *OperationsService) Rename(_ context.Context, oldPath string, newName string, actor model.AuditActor) (model.RenameResponse, error) {
	if strings.TrimSpace(oldPath) == "" {
		s.audit.Log("rename", actor, "failed", oldPath, map[string]any{"path": oldPath}, nil, "path is required")
		return model.RenameResponse{}, apierror.New("BAD_REQUEST", "path is required", "path", http.StatusBadRequest)
	}

	safeName, err := util.SanitizeFilename(newName, false)
	if err != nil {
		s.audit.Log("rename", actor, "failed", oldPath, map[string]any{"path": oldPath, "new_name": newName}, nil, err.Error())
		return model.RenameResponse{}, err
	}

	sourceResolved, err := s.store.Resolve(oldPath)
	if err != nil {
		s.audit.Log("rename", actor, "failed", oldPath, map[string]any{"path": oldPath, "new_name": safeName}, nil, err.Error())
		return model.RenameResponse{}, err
	}

	if _, err := os.Stat(sourceResolved); err != nil {
		if os.IsNotExist(err) {
			s.audit.Log("rename", actor, "failed", oldPath, map[string]any{"path": oldPath, "new_name": safeName}, nil, "path not found")
			return model.RenameResponse{}, apierror.New("NOT_FOUND", "path not found", oldPath, http.StatusNotFound)
		}
		s.audit.Log("rename", actor, "failed", oldPath, map[string]any{"path": oldPath, "new_name": safeName}, nil, err.Error())
		return model.RenameResponse{}, err
	}

	newAPIPath := normalizeAPIPath(filepath.Join(filepath.Dir(oldPath), safeName))
	newResolved, err := s.store.Resolve(newAPIPath)
	if err != nil {
		s.audit.Log("rename", actor, "failed", oldPath, map[string]any{"path": oldPath, "new_name": safeName}, nil, err.Error())
		return model.RenameResponse{}, err
	}

	if _, err := os.Stat(newResolved); err == nil {
		s.audit.Log("rename", actor, "failed", oldPath, map[string]any{"path": oldPath, "new_name": safeName}, nil, "target path already exists")
		return model.RenameResponse{}, apierror.New("ALREADY_EXISTS", "target path already exists", newAPIPath, http.StatusConflict)
	}

	if err := s.store.Rename(oldPath, newAPIPath); err != nil {
		s.audit.Log("rename", actor, "failed", oldPath, map[string]any{"path": oldPath, "new_name": safeName}, nil, err.Error())
		return model.RenameResponse{}, err
	}

	result := model.RenameResponse{OldPath: normalizeAPIPath(oldPath), NewPath: newAPIPath, Name: safeName}
	s.audit.Log("rename", actor, "success", normalizeAPIPath(oldPath), map[string]any{"path": normalizeAPIPath(oldPath)}, map[string]any{"path": newAPIPath}, "")
	return result, nil
}

func (s *OperationsService) Move(_ context.Context, sources []string, destination string, conflictPolicy string, actor model.AuditActor) (model.MoveResponse, error) {
	if len(sources) == 0 {
		s.audit.Log("move", actor, "failed", destination, map[string]any{"sources": sources, "destination": destination}, nil, "sources are required")
		return model.MoveResponse{}, apierror.New("BAD_REQUEST", "sources are required", "sources", http.StatusBadRequest)
	}

	normalizedPolicy, err := normalizeConflictPolicy(conflictPolicy)
	if err != nil {
		s.audit.Log("move", actor, "failed", destination, map[string]any{"sources": sources, "destination": destination, "conflict_policy": conflictPolicy}, nil, err.Error())
		return model.MoveResponse{}, err
	}

	destination = normalizeAPIPath(destination)
	if _, err := s.store.Resolve(destination); err != nil {
		s.audit.Log("move", actor, "failed", destination, map[string]any{"sources": sources, "destination": destination}, nil, err.Error())
		return model.MoveResponse{}, err
	}

	if err := s.store.MkdirAll(destination, 0o755); err != nil {
		s.audit.Log("move", actor, "failed", destination, map[string]any{"sources": sources, "destination": destination}, nil, err.Error())
		return model.MoveResponse{}, err
	}

	result := model.MoveResponse{Moved: []model.MoveCopyResult{}, Failed: []model.MoveCopyFailure{}}

	for _, source := range sources {
		source = normalizeAPIPath(source)
		if source == "/" {
			result.Failed = append(result.Failed, model.MoveCopyFailure{From: source, Reason: "root path cannot be moved"})
			s.audit.Log("move", actor, "failed", source, map[string]any{"from": source}, nil, "root path cannot be moved")
			continue
		}

		target := normalizeAPIPath(filepath.Join(destination, filepath.Base(source)))
		if source == target {
			result.Moved = append(result.Moved, model.MoveCopyResult{From: source, To: target})
			continue
		}

		resolvedTarget, skipped, resolveErr := resolveConflictTarget(s.store, target, normalizedPolicy)
		if resolveErr != nil {
			result.Failed = append(result.Failed, model.MoveCopyFailure{From: source, Reason: resolveErr.Error()})
			s.audit.Log("move", actor, "failed", source, map[string]any{"from": source, "to": target, "conflict_policy": normalizedPolicy}, nil, resolveErr.Error())
			continue
		}
		if skipped {
			reason := "skipped: target already exists"
			result.Failed = append(result.Failed, model.MoveCopyFailure{From: source, Reason: reason})
			s.audit.Log("move", actor, "failed", source, map[string]any{"from": source, "to": target, "conflict_policy": normalizedPolicy}, nil, reason)
			continue
		}

		if err := s.store.Rename(source, resolvedTarget); err != nil {
			result.Failed = append(result.Failed, model.MoveCopyFailure{From: source, Reason: err.Error()})
			s.audit.Log("move", actor, "failed", source, map[string]any{"from": source}, nil, err.Error())
			continue
		}

		result.Moved = append(result.Moved, model.MoveCopyResult{From: source, To: resolvedTarget})
		s.audit.Log("move", actor, "success", source, map[string]any{"from": source}, map[string]any{"to": resolvedTarget}, "")
	}

	return result, nil
}

func (s *OperationsService) Copy(_ context.Context, sources []string, destination string, conflictPolicy string, actor model.AuditActor) (model.CopyResponse, error) {
	if len(sources) == 0 {
		s.audit.Log("copy", actor, "failed", destination, map[string]any{"sources": sources, "destination": destination}, nil, "sources are required")
		return model.CopyResponse{}, apierror.New("BAD_REQUEST", "sources are required", "sources", http.StatusBadRequest)
	}

	normalizedPolicy, err := normalizeConflictPolicy(conflictPolicy)
	if err != nil {
		s.audit.Log("copy", actor, "failed", destination, map[string]any{"sources": sources, "destination": destination, "conflict_policy": conflictPolicy}, nil, err.Error())
		return model.CopyResponse{}, err
	}

	destination = normalizeAPIPath(destination)
	if _, err := s.store.Resolve(destination); err != nil {
		s.audit.Log("copy", actor, "failed", destination, map[string]any{"sources": sources, "destination": destination}, nil, err.Error())
		return model.CopyResponse{}, err
	}

	if err := s.store.MkdirAll(destination, 0o755); err != nil {
		s.audit.Log("copy", actor, "failed", destination, map[string]any{"sources": sources, "destination": destination}, nil, err.Error())
		return model.CopyResponse{}, err
	}

	result := model.CopyResponse{Copied: []model.MoveCopyResult{}, Failed: []model.MoveCopyFailure{}}

	for _, source := range sources {
		source = normalizeAPIPath(source)
		sourceResolved, err := s.store.Resolve(source)
		if err != nil {
			result.Failed = append(result.Failed, model.MoveCopyFailure{From: source, Reason: err.Error()})
			s.audit.Log("copy", actor, "failed", source, map[string]any{"from": source}, nil, err.Error())
			continue
		}

		if _, err := os.Stat(sourceResolved); err != nil {
			result.Failed = append(result.Failed, model.MoveCopyFailure{From: source, Reason: err.Error()})
			s.audit.Log("copy", actor, "failed", source, map[string]any{"from": source}, nil, err.Error())
			continue
		}

		target := normalizeAPIPath(filepath.Join(destination, filepath.Base(source)))
		resolvedTarget, skipped, resolveErr := resolveConflictTarget(s.store, target, normalizedPolicy)
		if resolveErr != nil {
			result.Failed = append(result.Failed, model.MoveCopyFailure{From: source, Reason: resolveErr.Error()})
			s.audit.Log("copy", actor, "failed", source, map[string]any{"from": source, "to": target, "conflict_policy": normalizedPolicy}, nil, resolveErr.Error())
			continue
		}
		if skipped {
			reason := "skipped: target already exists"
			result.Failed = append(result.Failed, model.MoveCopyFailure{From: source, Reason: reason})
			s.audit.Log("copy", actor, "failed", source, map[string]any{"from": source, "to": target, "conflict_policy": normalizedPolicy}, nil, reason)
			continue
		}

		resolvedTargetAbs, err := s.store.Resolve(resolvedTarget)
		if err != nil {
			result.Failed = append(result.Failed, model.MoveCopyFailure{From: source, Reason: err.Error()})
			s.audit.Log("copy", actor, "failed", source, map[string]any{"from": source}, nil, err.Error())
			continue
		}

		if err := copyRecursive(sourceResolved, resolvedTargetAbs); err != nil {
			result.Failed = append(result.Failed, model.MoveCopyFailure{From: source, Reason: err.Error()})
			s.audit.Log("copy", actor, "failed", source, map[string]any{"from": source}, nil, err.Error())
			continue
		}

		result.Copied = append(result.Copied, model.MoveCopyResult{From: source, To: resolvedTarget})
		s.audit.Log("copy", actor, "success", source, map[string]any{"from": source}, map[string]any{"to": resolvedTarget}, "")
	}

	return result, nil
}

func (s *OperationsService) Delete(_ context.Context, paths []string, actor model.AuditActor) (model.DeleteResponse, error) {
	if len(paths) == 0 {
		s.audit.Log("delete", actor, "failed", "", map[string]any{"paths": paths}, nil, "paths are required")
		return model.DeleteResponse{}, apierror.New("BAD_REQUEST", "paths are required", "paths", http.StatusBadRequest)
	}

	result := model.DeleteResponse{Deleted: []string{}, Failed: []model.DeleteFailure{}}

	for _, path := range paths {
		path = normalizeAPIPath(path)
		if path == "/" {
			result.Failed = append(result.Failed, model.DeleteFailure{Path: path, Reason: "root path cannot be deleted"})
			s.audit.Log("delete", actor, "failed", path, map[string]any{"path": path}, nil, "root path cannot be deleted")
			continue
		}

		record, err := s.trash.SoftDelete(path, actor)
		if err != nil {
			result.Failed = append(result.Failed, model.DeleteFailure{Path: path, Reason: err.Error()})
			s.audit.Log("delete", actor, "failed", path, map[string]any{"path": path}, nil, err.Error())
			continue
		}

		result.Deleted = append(result.Deleted, path)
		s.audit.Log("delete", actor, "success", path, map[string]any{"path": path}, map[string]any{"trash_id": record.ID, "deleted_at": record.DeletedAt}, "")
	}

	return result, nil
}

func (s *OperationsService) Restore(_ context.Context, paths []string, actor model.AuditActor) (model.RestoreResponse, error) {
	if len(paths) == 0 {
		s.audit.Log("restore", actor, "failed", "", map[string]any{"paths": paths}, nil, "paths are required")
		return model.RestoreResponse{}, apierror.New("BAD_REQUEST", "paths are required", "paths", http.StatusBadRequest)
	}

	result := model.RestoreResponse{Restored: []string{}, Failed: []model.RestoreFailure{}}

	for _, path := range paths {
		path = normalizeAPIPath(path)
		if path == "/" {
			result.Failed = append(result.Failed, model.RestoreFailure{Path: path, Reason: "root path cannot be restored"})
			s.audit.Log("restore", actor, "failed", path, map[string]any{"path": path}, nil, "root path cannot be restored")
			continue
		}

		record, err := s.trash.RestoreLatest(path, actor)
		if err != nil {
			reason := err.Error()
			if os.IsNotExist(err) {
				reason = "no trashed version found"
			}
			result.Failed = append(result.Failed, model.RestoreFailure{Path: path, Reason: reason})
			s.audit.Log("restore", actor, "failed", path, map[string]any{"path": path}, nil, reason)
			continue
		}

		result.Restored = append(result.Restored, path)
		s.audit.Log("restore", actor, "success", path, map[string]any{"trash_id": record.ID}, map[string]any{"path": path, "restored_at": record.RestoredAt}, "")
	}

	return result, nil
}

func (s *OperationsService) ListTrash(_ context.Context, includeRestored bool) ([]model.TrashRecord, error) {
	return s.trash.List(includeRestored)
}

func (s *OperationsService) PermanentDeleteTrash(_ context.Context, trashID string, actor model.AuditActor) error {
	if err := s.trash.PermanentDelete(trashID); err != nil {
		s.audit.Log("permanent_delete", actor, "failed", trashID, map[string]any{"trash_id": trashID}, nil, err.Error())
		return err
	}

	s.audit.Log("permanent_delete", actor, "success", trashID, map[string]any{"trash_id": trashID}, nil, "")
	return nil
}

func (s *OperationsService) EmptyTrash(_ context.Context, actor model.AuditActor) (int, error) {
	count, err := s.trash.EmptyTrash()
	if err != nil {
		s.audit.Log("empty_trash", actor, "failed", "", nil, nil, err.Error())
		return count, err
	}

	s.audit.Log("empty_trash", actor, "success", "", nil, map[string]any{"deleted_count": count}, "")
	return count, nil
}

func copyRecursive(source string, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}

	if info.IsDir() {
		if err := os.MkdirAll(target, info.Mode()); err != nil {
			return err
		}

		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			if err := copyRecursive(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
				return err
			}
		}

		return nil
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	targetFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer targetFile.Close()

	_, err = io.Copy(targetFile, sourceFile)
	return err
}
