package service

import (
"context"
"net/http"
"time"

"github.com/google/uuid"

"go-file-explorer/internal/model"
"go-file-explorer/internal/repository"
"go-file-explorer/pkg/apierror"
)

type ShareService struct {
shareRepo *repository.ShareRepository
}

func NewShareService(shareRepo *repository.ShareRepository) *ShareService {
return &ShareService{shareRepo: shareRepo}
}

func (s *ShareService) Create(path string, actor string, expiresIn string) (model.ShareRecord, error) {
if path == "" {
return model.ShareRecord{}, apierror.New("BAD_REQUEST", "path is required", "", http.StatusBadRequest)
}

ctx := context.Background()

duration, err := time.ParseDuration(expiresIn)
if err != nil || duration <= 0 {
duration = 24 * time.Hour
}

now := time.Now().UTC()
record := model.ShareRecord{
ID:        uuid.NewString(),
Token:     uuid.NewString(),
Path:      path,
CreatedBy: actor,
CreatedAt: now.Format(time.RFC3339Nano),
ExpiresAt: now.Add(duration).Format(time.RFC3339Nano),
}

if err := s.shareRepo.Create(ctx, record); err != nil {
return model.ShareRecord{}, err
}

return record, nil
}

func (s *ShareService) List(userID string) ([]model.ShareRecord, error) {
ctx := context.Background()
return s.shareRepo.ListByUser(ctx, userID)
}

func (s *ShareService) Revoke(shareID string, userID string) error {
ctx := context.Background()
return s.shareRepo.Revoke(ctx, shareID, userID)
}

func (s *ShareService) ResolveToken(token string) (model.ShareRecord, error) {
ctx := context.Background()
return s.shareRepo.ResolveToken(ctx, token)
}
