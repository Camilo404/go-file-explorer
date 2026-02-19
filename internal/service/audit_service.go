package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go-file-explorer/internal/model"
	"go-file-explorer/pkg/apierror"
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

func (s *AuditService) Query(query model.AuditQuery) ([]model.AuditEntry, model.Meta, error) {
	if query.Page < 1 {
		query.Page = 1
	}
	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > 200 {
		query.Limit = 200
	}

	from, err := parseOptionalAuditTime(query.From)
	if err != nil {
		return nil, model.Meta{}, apierror.New("BAD_REQUEST", "invalid 'from' datetime format", query.From, http.StatusBadRequest)
	}

	to, err := parseOptionalAuditTime(query.To)
	if err != nil {
		return nil, model.Meta{}, apierror.New("BAD_REQUEST", "invalid 'to' datetime format", query.To, http.StatusBadRequest)
	}

	action := strings.ToLower(strings.TrimSpace(query.Action))
	status := strings.ToLower(strings.TrimSpace(query.Status))
	actorID := strings.TrimSpace(query.ActorID)
	pathFilter := strings.TrimSpace(query.Path)

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.filePath)
	if err != nil {
		return nil, model.Meta{}, err
	}
	defer f.Close()

	items := make([]model.AuditEntry, 0, 128)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var entry model.AuditEntry
		if unmarshalErr := json.Unmarshal([]byte(line), &entry); unmarshalErr != nil {
			continue
		}

		if action != "" && strings.ToLower(strings.TrimSpace(entry.Action)) != action {
			continue
		}
		if status != "" && strings.ToLower(strings.TrimSpace(entry.Status)) != status {
			continue
		}
		if actorID != "" && strings.TrimSpace(entry.Actor.UserID) != actorID {
			continue
		}
		if pathFilter != "" && !strings.Contains(strings.ToLower(entry.Resource), strings.ToLower(pathFilter)) {
			continue
		}

		at, timeErr := parseAuditTime(entry.OccurredAt)
		if timeErr != nil {
			continue
		}

		if !from.IsZero() && at.Before(from) {
			continue
		}
		if !to.IsZero() && at.After(to) {
			continue
		}

		items = append(items, entry)
	}

	if scanErr := scanner.Err(); scanErr != nil {
		return nil, model.Meta{}, scanErr
	}

	sort.SliceStable(items, func(i int, j int) bool {
		left, leftErr := parseAuditTime(items[i].OccurredAt)
		right, rightErr := parseAuditTime(items[j].OccurredAt)
		if leftErr != nil || rightErr != nil {
			return items[i].OccurredAt > items[j].OccurredAt
		}
		return left.After(right)
	})

	total := len(items)
	start := (query.Page - 1) * query.Limit
	if start > total {
		start = total
	}
	end := start + query.Limit
	if end > total {
		end = total
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + query.Limit - 1) / query.Limit
	}

	meta := model.Meta{Page: query.Page, Limit: query.Limit, Total: total, TotalPages: totalPages}
	return items[start:end], meta, nil
}

func parseOptionalAuditTime(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, nil
	}

	return parseAuditTime(trimmed)
}

func parseAuditTime(raw string) (time.Time, error) {
	if value, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return value.UTC(), nil
	}

	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, err
	}

	return value.UTC(), nil
}
