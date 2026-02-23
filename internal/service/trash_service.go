package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/repository"
	"go-file-explorer/internal/storage"
)

type TrashService struct {
	store         storage.Storage
	trashRoot     string
	thumbnailRoot string
	trashRepo     *repository.TrashRepository
}

func NewTrashService(store storage.Storage, trashRoot string, trashRepo *repository.TrashRepository) (*TrashService, error) {
	if err := os.MkdirAll(trashRoot, 0o755); err != nil {
		return nil, fmt.Errorf("prepare trash directory: %w", err)
	}

	return &TrashService{store: store, trashRoot: trashRoot, thumbnailRoot: "./data/.thumbnails", trashRepo: trashRepo}, nil
}

func (s *TrashService) SetThumbnailRoot(thumbnailRoot string) {
	trimmed := strings.TrimSpace(thumbnailRoot)
	if trimmed == "" {
		return
	}
	s.thumbnailRoot = trimmed
}

func (s *TrashService) SoftDelete(apiPath string, actor model.AuditActor) (model.TrashRecord, error) {
	ctx := context.Background()

	resolved, err := s.store.Resolve(apiPath)
	if err != nil {
		return model.TrashRecord{}, err
	}

	if _, err := os.Stat(resolved); err != nil {
		return model.TrashRecord{}, err
	}

	record := model.TrashRecord{
		ID:           uuid.NewString(),
		OriginalPath: apiPath,
		TrashName:    uuid.NewString() + "_" + filepath.Base(apiPath),
		DeletedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		DeletedBy:    actor,
	}

	trashPath := filepath.Join(s.trashRoot, record.TrashName)
	if err := movePath(resolved, trashPath); err != nil {
		return model.TrashRecord{}, fmt.Errorf("move to trash %q: %w", apiPath, err)
	}

	if err := s.trashRepo.Create(ctx, record); err != nil {
		_ = movePath(trashPath, resolved)
		return model.TrashRecord{}, err
	}

	return record, nil
}

func (s *TrashService) RestoreLatest(apiPath string, actor model.AuditActor) (model.TrashRecord, error) {
	ctx := context.Background()

	record, err := s.trashRepo.FindLatestByPath(ctx, apiPath)
	if err != nil {
		return model.TrashRecord{}, err
	}

	targetResolved, err := s.store.Resolve(apiPath)
	if err != nil {
		return model.TrashRecord{}, err
	}

	if _, err := os.Stat(targetResolved); err == nil {
		return model.TrashRecord{}, fmt.Errorf("%w: target already exists", model.ErrPathConflict)
	} else if !os.IsNotExist(err) {
		return model.TrashRecord{}, err
	}

	if err := os.MkdirAll(filepath.Dir(targetResolved), 0o755); err != nil {
		return model.TrashRecord{}, err
	}

	trashPath := filepath.Join(s.trashRoot, record.TrashName)
	if err := movePath(trashPath, targetResolved); err != nil {
		return model.TrashRecord{}, fmt.Errorf("restore %q: %w", apiPath, err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.trashRepo.MarkRestored(ctx, record.ID, actor); err != nil {
		_ = movePath(targetResolved, trashPath)
		return model.TrashRecord{}, err
	}

	record.RestoredAt = now
	record.RestoredBy = actor
	return record, nil
}

func (s *TrashService) List(includeRestored bool) ([]model.TrashRecord, error) {
	ctx := context.Background()
	return s.trashRepo.List(ctx, includeRestored)
}

func (s *TrashService) PermanentDelete(trashID string) error {
	ctx := context.Background()

	record, err := s.trashRepo.FindByID(ctx, trashID)
	if err != nil {
		return err
	}

	trashPath := filepath.Join(s.trashRoot, record.TrashName)
	affectedPaths, collectErr := collectOriginalFilePathsForTrashRecord(trashPath, record.OriginalPath)
	if collectErr != nil && !os.IsNotExist(collectErr) {
		return collectErr
	}

	if err := os.RemoveAll(trashPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove trash file %q: %w", trashID, err)
	}

	for _, apiPath := range affectedPaths {
		if err := s.removeThumbnailsForPath(apiPath); err != nil {
			return err
		}
	}

	return s.trashRepo.Delete(ctx, trashID)
}

func (s *TrashService) EmptyTrash() (int, error) {
	ctx := context.Background()

	records, err := s.trashRepo.List(ctx, false)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, record := range records {
		trashPath := filepath.Join(s.trashRoot, record.TrashName)
		affectedPaths, collectErr := collectOriginalFilePathsForTrashRecord(trashPath, record.OriginalPath)
		if collectErr != nil && !os.IsNotExist(collectErr) {
			continue
		}

		if removeErr := os.RemoveAll(trashPath); removeErr != nil && !os.IsNotExist(removeErr) {
			continue
		}

		cleanupFailed := false
		for _, apiPath := range affectedPaths {
			if thumbErr := s.removeThumbnailsForPath(apiPath); thumbErr != nil {
				cleanupFailed = true
				break
			}
		}

		if cleanupFailed {
			continue
		}
		count++
	}

	if _, err := s.trashRepo.DeleteAllNotRestored(ctx); err != nil {
		return count, err
	}

	return count, nil
}

func (s *TrashService) removeThumbnailsForPath(apiPath string) error {
	resolved, err := s.store.Resolve(apiPath)
	if err != nil {
		return nil
	}

	for size := 32; size <= 2048; size++ {
		thumbPath := filepath.Join(s.thumbnailRoot, thumbnailFileName(resolved, size))
		if removeErr := os.Remove(thumbPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("remove thumbnail %q: %w", thumbPath, removeErr)
		}
	}

	return nil
}

func thumbnailFileName(resolvedPath string, size int) string {
	hash := sha256.Sum256([]byte(resolvedPath + "|" + strconv.Itoa(size)))
	return hex.EncodeToString(hash[:]) + ".jpg"
}

func collectOriginalFilePathsForTrashRecord(trashPath string, originalAPIPath string) ([]string, error) {
	info, err := os.Stat(trashPath)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return []string{normalizeAPIPathForTrash(originalAPIPath)}, nil
	}

	baseAPIPath := normalizeAPIPathForTrash(originalAPIPath)
	paths := make([]string, 0)
	walkErr := filepath.WalkDir(trashPath, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		rel, relErr := filepath.Rel(trashPath, current)
		if relErr != nil {
			return relErr
		}

		relSlash := filepath.ToSlash(rel)
		paths = append(paths, normalizeAPIPathForTrash(path.Join(baseAPIPath, relSlash)))
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	return paths, nil
}

func normalizeAPIPathForTrash(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "/"
	}

	cleaned := path.Clean("/" + strings.TrimPrefix(strings.ReplaceAll(trimmed, "\\", "/"), "/"))
	if cleaned == "." {
		return "/"
	}

	return cleaned
}

func movePath(source string, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}

	if err := os.Rename(source, destination); err == nil {
		return nil
	} else if !isCrossDeviceRenameError(err) {
		return err
	}

	if err := copyPathRecursive(source, destination); err != nil {
		return err
	}

	if err := os.RemoveAll(source); err != nil {
		return err
	}

	return nil
}

func isCrossDeviceRenameError(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) && strings.Contains(strings.ToLower(linkErr.Err.Error()), "cross-device") {
		return true
	}

	return strings.Contains(strings.ToLower(err.Error()), "cross-device")
}

func copyPathRecursive(source string, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return copyFile(source, destination, info.Mode())
	}

	if err := os.MkdirAll(destination, info.Mode().Perm()); err != nil {
		return err
	}

	return filepath.WalkDir(source, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, relErr := filepath.Rel(source, current)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}

		target := filepath.Join(destination, rel)
		entryInfo, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}

		if entry.IsDir() {
			return os.MkdirAll(target, entryInfo.Mode().Perm())
		}

		return copyFile(current, target, entryInfo.Mode())
	})
}

func copyFile(source string, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}

	output, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}

	return nil
}
