package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/storage"
	"go-file-explorer/internal/util"
	"go-file-explorer/pkg/apierror"
)

// ── Upload session (in-memory metadata) ──────────────────────────

type uploadSession struct {
	uploadID       string
	fileName       string
	destination    string
	conflictPolicy string
	totalChunks    int
	chunkSize      int64
	fileSize       int64
	receivedChunks int
	received       []bool // tracks which chunks have been written
	tempFilePath   string
	createdAt      time.Time
	mu             sync.Mutex // per-session lock for chunk writes
}

// ── Service ──────────────────────────────────────────────────────

type ChunkedUploadService struct {
	store            *storage.Storage
	tempDir          string
	allowedMIMETypes map[string]struct{}

	mu       sync.RWMutex
	sessions map[string]*uploadSession
}

func NewChunkedUploadService(store *storage.Storage, tempDir string, allowedMIMETypes []string) (*ChunkedUploadService, error) {
	if strings.TrimSpace(tempDir) == "" {
		tempDir = "./data/.chunks"
	}

	abs, err := filepath.Abs(tempDir)
	if err != nil {
		return nil, fmt.Errorf("resolve chunk temp dir: %w", err)
	}

	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create chunk temp dir: %w", err)
	}

	allowed := make(map[string]struct{}, len(allowedMIMETypes))
	for _, mt := range allowedMIMETypes {
		trimmed := strings.TrimSpace(strings.ToLower(mt))
		if trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}

	return &ChunkedUploadService{
		store:            store,
		tempDir:          abs,
		allowedMIMETypes: allowed,
		sessions:         make(map[string]*uploadSession),
	}, nil
}

// ── Init ─────────────────────────────────────────────────────────

func (s *ChunkedUploadService) InitUpload(_ context.Context, req model.ChunkedUploadInitRequest) (model.ChunkedUploadInitResponse, error) {
	if strings.TrimSpace(req.FileName) == "" {
		return model.ChunkedUploadInitResponse{}, apierror.New("BAD_REQUEST", "file_name is required", "", http.StatusBadRequest)
	}

	safeName, err := util.SanitizeFilename(req.FileName, false)
	if err != nil {
		return model.ChunkedUploadInitResponse{}, err
	}

	if req.FileSize <= 0 {
		return model.ChunkedUploadInitResponse{}, apierror.New("BAD_REQUEST", "file_size must be positive", "", http.StatusBadRequest)
	}

	if req.ChunkSize <= 0 {
		return model.ChunkedUploadInitResponse{}, apierror.New("BAD_REQUEST", "chunk_size must be positive", "", http.StatusBadRequest)
	}

	totalChunks := int((req.FileSize + req.ChunkSize - 1) / req.ChunkSize)

	uploadID, err := generateUploadID()
	if err != nil {
		return model.ChunkedUploadInitResponse{}, fmt.Errorf("generate upload id: %w", err)
	}

	tempPath := filepath.Join(s.tempDir, uploadID+".part")

	// Pre-allocate the temp file (create empty, we will seek-write later).
	f, err := os.Create(tempPath)
	if err != nil {
		return model.ChunkedUploadInitResponse{}, fmt.Errorf("create temp file: %w", err)
	}
	f.Close()

	destination := strings.TrimSpace(req.Destination)
	if destination == "" {
		destination = "/"
	}

	sess := &uploadSession{
		uploadID:       uploadID,
		fileName:       safeName,
		destination:    destination,
		conflictPolicy: req.ConflictPolicy,
		totalChunks:    totalChunks,
		chunkSize:      req.ChunkSize,
		fileSize:       req.FileSize,
		receivedChunks: 0,
		received:       make([]bool, totalChunks),
		tempFilePath:   tempPath,
		createdAt:      time.Now(),
	}

	s.mu.Lock()
	s.sessions[uploadID] = sess
	s.mu.Unlock()

	slog.Info("chunked upload initiated",
		"upload_id", uploadID,
		"file_name", safeName,
		"file_size", req.FileSize,
		"chunk_size", req.ChunkSize,
		"total_chunks", totalChunks,
	)

	return model.ChunkedUploadInitResponse{
		UploadID:    uploadID,
		ChunkSize:   req.ChunkSize,
		TotalChunks: totalChunks,
	}, nil
}

// ── Write chunk ──────────────────────────────────────────────────

func (s *ChunkedUploadService) WriteChunk(_ context.Context, uploadID string, chunkIndex int, reader io.Reader) (model.ChunkedUploadChunkResponse, error) {
	s.mu.RLock()
	sess, ok := s.sessions[uploadID]
	s.mu.RUnlock()

	if !ok {
		return model.ChunkedUploadChunkResponse{}, apierror.New("NOT_FOUND", "upload session not found", uploadID, http.StatusNotFound)
	}

	if chunkIndex < 0 || chunkIndex >= sess.totalChunks {
		return model.ChunkedUploadChunkResponse{}, apierror.New(
			"BAD_REQUEST",
			fmt.Sprintf("chunk_index must be between 0 and %d", sess.totalChunks-1),
			"",
			http.StatusBadRequest,
		)
	}

	// Per-session lock: serialises writes to the same temp file.
	sess.mu.Lock()
	defer sess.mu.Unlock()

	offset := int64(chunkIndex) * sess.chunkSize

	f, err := os.OpenFile(sess.tempFilePath, os.O_WRONLY, 0o644)
	if err != nil {
		return model.ChunkedUploadChunkResponse{}, fmt.Errorf("open temp file for chunk write: %w", err)
	}
	defer f.Close()

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return model.ChunkedUploadChunkResponse{}, fmt.Errorf("seek to chunk offset: %w", err)
	}

	// 32 KB buffer — same as the standard upload path.
	buf := make([]byte, 32*1024)
	if _, err := io.CopyBuffer(f, reader, buf); err != nil {
		return model.ChunkedUploadChunkResponse{}, fmt.Errorf("write chunk data: %w", err)
	}

	// Track idempotent reception.
	if !sess.received[chunkIndex] {
		sess.received[chunkIndex] = true
		sess.receivedChunks++
	}

	return model.ChunkedUploadChunkResponse{
		UploadID:       uploadID,
		ChunkIndex:     chunkIndex,
		ChunksReceived: sess.receivedChunks,
	}, nil
}

// ── Complete ─────────────────────────────────────────────────────

func (s *ChunkedUploadService) CompleteUpload(_ context.Context, uploadID string) (model.UploadItem, error) {
	s.mu.RLock()
	sess, ok := s.sessions[uploadID]
	s.mu.RUnlock()

	if !ok {
		return model.UploadItem{}, apierror.New("NOT_FOUND", "upload session not found", uploadID, http.StatusNotFound)
	}

	sess.mu.Lock()
	defer sess.mu.Unlock()

	if sess.receivedChunks != sess.totalChunks {
		missing := make([]int, 0)
		for i, got := range sess.received {
			if !got {
				missing = append(missing, i)
				if len(missing) >= 10 {
					break // show max 10
				}
			}
		}
		return model.UploadItem{}, apierror.New(
			"BAD_REQUEST",
			fmt.Sprintf("upload incomplete: received %d of %d chunks", sess.receivedChunks, sess.totalChunks),
			fmt.Sprintf("missing chunks (first up to 10): %v", missing),
			http.StatusBadRequest,
		)
	}

	// MIME sniffing on the first 512 bytes of the assembled file.
	f, err := os.Open(sess.tempFilePath)
	if err != nil {
		return model.UploadItem{}, fmt.Errorf("open temp for MIME check: %w", err)
	}

	sniffBuf := make([]byte, 512)
	n, readErr := io.ReadFull(f, sniffBuf)
	f.Close()
	if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
		return model.UploadItem{}, fmt.Errorf("read MIME header: %w", readErr)
	}

	detectedMIME := http.DetectContentType(sniffBuf[:n])
	if !s.isAllowedMIME(detectedMIME) {
		// Clean up the bad file.
		os.Remove(sess.tempFilePath)
		s.removeSession(uploadID)
		return model.UploadItem{}, apierror.New("UNSUPPORTED_TYPE", "file MIME type is not allowed", detectedMIME, http.StatusUnsupportedMediaType)
	}

	// Resolve destination path + conflict.
	destPath := normalizeAPIPath(filepath.Join(normalizeAPIPath(sess.destination), sess.fileName))
	if err := s.store.MkdirAll(normalizeAPIPath(sess.destination), 0o755); err != nil {
		return model.UploadItem{}, err
	}

	targetPath, skipped, err := resolveConflictTarget(s.store, destPath, sess.conflictPolicy)
	if err != nil {
		return model.UploadItem{}, err
	}
	if skipped {
		os.Remove(sess.tempFilePath)
		s.removeSession(uploadID)
		return model.UploadItem{}, apierror.New("CONFLICT", "target already exists and conflict_policy=skip", sess.fileName, http.StatusConflict)
	}

	// Resolve the final absolute path on disk.
	resolvedTarget, err := s.store.Resolve(targetPath)
	if err != nil {
		return model.UploadItem{}, err
	}

	// Ensure parent directories exist.
	if err := os.MkdirAll(filepath.Dir(resolvedTarget), 0o755); err != nil {
		return model.UploadItem{}, fmt.Errorf("create destination dir: %w", err)
	}

	// Atomic rename (same partition) — no extra I/O on HDD.
	if err := os.Rename(sess.tempFilePath, resolvedTarget); err != nil {
		return model.UploadItem{}, fmt.Errorf("move temp file to destination: %w", err)
	}

	// Get final size from disk.
	info, err := os.Stat(resolvedTarget)
	if err != nil {
		return model.UploadItem{}, fmt.Errorf("stat final file: %w", err)
	}

	s.removeSession(uploadID)

	slog.Info("chunked upload completed",
		"upload_id", uploadID,
		"file", targetPath,
		"size", info.Size(),
	)

	return model.UploadItem{
		Name:     sess.fileName,
		Path:     targetPath,
		Size:     info.Size(),
		MimeType: detectedMIME,
	}, nil
}

// ── Abort ────────────────────────────────────────────────────────

func (s *ChunkedUploadService) AbortUpload(_ context.Context, uploadID string) error {
	s.mu.RLock()
	sess, ok := s.sessions[uploadID]
	s.mu.RUnlock()

	if !ok {
		return apierror.New("NOT_FOUND", "upload session not found", uploadID, http.StatusNotFound)
	}

	sess.mu.Lock()
	tempPath := sess.tempFilePath
	sess.mu.Unlock()

	os.Remove(tempPath)
	s.removeSession(uploadID)

	slog.Info("chunked upload aborted", "upload_id", uploadID)
	return nil
}

// ── Cleanup expired sessions ─────────────────────────────────────

func (s *ChunkedUploadService) CleanupExpired(maxAge time.Duration) {
	now := time.Now()

	// 1. Remove expired sessions from the in-memory map.
	s.mu.Lock()
	var expired []string
	for id, sess := range s.sessions {
		if now.Sub(sess.createdAt) > maxAge {
			expired = append(expired, id)
		}
	}
	for _, id := range expired {
		sess := s.sessions[id]
		os.Remove(sess.tempFilePath)
		delete(s.sessions, id)
	}
	s.mu.Unlock()

	if len(expired) > 0 {
		slog.Info("cleaned up expired upload sessions", "count", len(expired))
	}

	// 2. Scan the temp directory for orphan .part files not tracked in the map.
	entries, err := os.ReadDir(s.tempDir)
	if err != nil {
		slog.Warn("chunk cleanup: failed to read temp dir", "error", err)
		return
	}

	orphansRemoved := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".part") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if now.Sub(info.ModTime()) > maxAge {
			fullPath := filepath.Join(s.tempDir, entry.Name())

			// If it's still tracked (shouldn't be after step 1, but be safe), skip.
			id := strings.TrimSuffix(entry.Name(), ".part")
			s.mu.RLock()
			_, tracked := s.sessions[id]
			s.mu.RUnlock()
			if tracked {
				continue
			}

			if err := os.Remove(fullPath); err == nil {
				orphansRemoved++
			}
		}
	}

	if orphansRemoved > 0 {
		slog.Info("cleaned up orphan chunk files", "count", orphansRemoved)
	}
}

// StartCleanupTicker runs CleanupExpired on a regular interval until ctx is cancelled.
func (s *ChunkedUploadService) StartCleanupTicker(ctx context.Context, maxAge time.Duration) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	// Run once on startup to clear stale files from a previous run.
	s.CleanupExpired(maxAge)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.CleanupExpired(maxAge)
		}
	}
}

// ── Helpers ──────────────────────────────────────────────────────

func (s *ChunkedUploadService) removeSession(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func (s *ChunkedUploadService) isAllowedMIME(mime string) bool {
	if len(s.allowedMIMETypes) == 0 {
		return true // no allow-list configured → accept everything
	}

	// Check exact match first.
	base := strings.SplitN(mime, ";", 2)[0]
	base = strings.TrimSpace(strings.ToLower(base))

	if _, ok := s.allowedMIMETypes[base]; ok {
		return true
	}

	// Check wildcard, e.g. "image/*".
	parts := strings.SplitN(base, "/", 2)
	if len(parts) == 2 {
		wildcard := parts[0] + "/*"
		if _, ok := s.allowedMIMETypes[wildcard]; ok {
			return true
		}
	}

	return false
}

func generateUploadID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
