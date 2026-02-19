package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go-file-explorer/internal/model"
)

type AuditService struct {
	filePath string
	mu       sync.Mutex
}

func NewAuditService(filePath string) (*AuditService, error) {
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, fmt.Errorf("prepare audit directory: %w", err)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		if err := os.WriteFile(filePath, []byte{}, 0o644); err != nil {
			return nil, fmt.Errorf("initialize audit file: %w", err)
		}
	}

	return &AuditService{filePath: filePath}, nil
}

func (s *AuditService) Log(action string, actor model.AuditActor, status string, resource string, before any, after any, errText string) {
	if s == nil {
		return
	}

	entry := model.AuditEntry{
		Action:     action,
		OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		Actor:      actor,
		Status:     status,
		Resource:   resource,
		Before:     before,
		After:      after,
		Error:      errText,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	_, _ = f.Write(append(data, '\n'))
}
