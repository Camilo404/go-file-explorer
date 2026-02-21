package service

import (
"context"
"log/slog"
"time"

"go-file-explorer/internal/model"
"go-file-explorer/internal/repository"
)

type AuditService struct {
auditRepo *repository.AuditRepository
}

func NewAuditService(auditRepo *repository.AuditRepository) *AuditService {
return &AuditService{auditRepo: auditRepo}
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

ctx := context.Background()
if err := s.auditRepo.Log(ctx, entry); err != nil {
slog.Error("failed to log audit entry to database", "error", err)
}
}

func (s *AuditService) Query(query model.AuditQuery) ([]model.AuditEntry, model.Meta, error) {
ctx := context.Background()
return s.auditRepo.Query(ctx, query)
}
