package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/storage"
)

type TrashRecord struct {
	ID           string           `json:"id"`
	OriginalPath string           `json:"original_path"`
	TrashName    string           `json:"trash_name"`
	DeletedAt    string           `json:"deleted_at"`
	DeletedBy    model.AuditActor `json:"deleted_by"`
	RestoredAt   string           `json:"restored_at,omitempty"`
	RestoredBy   model.AuditActor `json:"restored_by,omitempty"`
}

type trashIndex struct {
	Records []TrashRecord `json:"records"`
}

type TrashService struct {
	store     *storage.Storage
	trashRoot string
	indexFile string
	mu        sync.Mutex
}

func NewTrashService(store *storage.Storage, trashRoot string, indexFile string) (*TrashService, error) {
	if err := os.MkdirAll(trashRoot, 0o755); err != nil {
		return nil, fmt.Errorf("prepare trash directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(indexFile), 0o755); err != nil {
		return nil, fmt.Errorf("prepare trash index directory: %w", err)
	}
	if _, err := os.Stat(indexFile); os.IsNotExist(err) {
		initial, marshalErr := json.Marshal(trashIndex{Records: []TrashRecord{}})
		if marshalErr != nil {
			return nil, marshalErr
		}
		if writeErr := os.WriteFile(indexFile, initial, 0o644); writeErr != nil {
			return nil, fmt.Errorf("initialize trash index file: %w", writeErr)
		}
	}

	return &TrashService{store: store, trashRoot: trashRoot, indexFile: indexFile}, nil
}

func (s *TrashService) SoftDelete(apiPath string, actor model.AuditActor) (TrashRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	resolved, err := s.store.Resolve(apiPath)
	if err != nil {
		return TrashRecord{}, err
	}

	if _, err := os.Stat(resolved); err != nil {
		return TrashRecord{}, err
	}

	record := TrashRecord{
		ID:           uuid.NewString(),
		OriginalPath: apiPath,
		TrashName:    uuid.NewString() + "_" + filepath.Base(apiPath),
		DeletedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		DeletedBy:    actor,
	}

	trashPath := filepath.Join(s.trashRoot, record.TrashName)
	if err := os.Rename(resolved, trashPath); err != nil {
		return TrashRecord{}, fmt.Errorf("move to trash %q: %w", apiPath, err)
	}

	index, err := s.loadIndex()
	if err != nil {
		return TrashRecord{}, err
	}

	index.Records = append(index.Records, record)
	if err := s.saveIndex(index); err != nil {
		return TrashRecord{}, err
	}

	return record, nil
}

func (s *TrashService) RestoreLatest(apiPath string, actor model.AuditActor) (TrashRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.loadIndex()
	if err != nil {
		return TrashRecord{}, err
	}

	selected := -1
	for idx := len(index.Records) - 1; idx >= 0; idx-- {
		record := index.Records[idx]
		if record.OriginalPath == apiPath && record.RestoredAt == "" {
			selected = idx
			break
		}
	}

	if selected < 0 {
		return TrashRecord{}, os.ErrNotExist
	}

	record := index.Records[selected]
	targetResolved, err := s.store.Resolve(apiPath)
	if err != nil {
		return TrashRecord{}, err
	}

	if _, err := os.Stat(targetResolved); err == nil {
		return TrashRecord{}, fmt.Errorf("target already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return TrashRecord{}, err
	}

	if err := os.MkdirAll(filepath.Dir(targetResolved), 0o755); err != nil {
		return TrashRecord{}, err
	}

	trashPath := filepath.Join(s.trashRoot, record.TrashName)
	if err := os.Rename(trashPath, targetResolved); err != nil {
		return TrashRecord{}, fmt.Errorf("restore %q: %w", apiPath, err)
	}

	record.RestoredAt = time.Now().UTC().Format(time.RFC3339Nano)
	record.RestoredBy = actor
	index.Records[selected] = record

	if err := s.saveIndex(index); err != nil {
		return TrashRecord{}, err
	}

	return record, nil
}

func (s *TrashService) List(includeRestored bool) ([]TrashRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.loadIndex()
	if err != nil {
		return nil, err
	}

	records := make([]TrashRecord, 0, len(index.Records))
	for idx := len(index.Records) - 1; idx >= 0; idx-- {
		record := index.Records[idx]
		if !includeRestored && record.RestoredAt != "" {
			continue
		}
		records = append(records, record)
	}

	return records, nil
}

func (s *TrashService) loadIndex() (trashIndex, error) {
	data, err := os.ReadFile(s.indexFile)
	if err != nil {
		return trashIndex{}, fmt.Errorf("read trash index: %w", err)
	}

	if len(data) == 0 {
		return trashIndex{Records: []TrashRecord{}}, nil
	}

	var index trashIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return trashIndex{}, fmt.Errorf("parse trash index: %w", err)
	}

	if index.Records == nil {
		index.Records = []TrashRecord{}
	}

	return index, nil
}

func (s *TrashService) saveIndex(index trashIndex) error {
	data, err := json.Marshal(index)
	if err != nil {
		return err
	}

	tmpPath := s.indexFile + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write trash index temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.indexFile); err != nil {
		return fmt.Errorf("replace trash index file: %w", err)
	}

	return nil
}
