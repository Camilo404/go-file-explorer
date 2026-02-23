package service

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"go-file-explorer/internal/event"
	"go-file-explorer/internal/storage"
)

func TestFileService_Upload(t *testing.T) {
	allowedMIMEs := []string{"text/plain", "image/png"}

	t.Run("success upload new file", func(t *testing.T) {
		mockStore := new(storage.MockStorage)
		mockBus := event.NewBus()
		svc := NewFileService(mockStore, allowedMIMEs, "/tmp/thumbnails", mockBus)

		// Inputs
		filename := "test.txt"
		destination := "/docs"
		content := "hello world"
		reader := strings.NewReader(content)

		// Mock expectations
		// 1. MkdirAll
		mockStore.On("MkdirAll", "/docs", fs.FileMode(0o755)).Return(nil)

		// 2. resolveConflictTarget calls Stat to check existence
		// Simulate file does not exist (returns IsNotExist error)
		mockStore.On("Stat", "/docs/test.txt").Return(nil, os.ErrNotExist)

		// 3. OpenForWrite
		mockFile, _ := os.CreateTemp("", "mock_upload_")
		defer os.Remove(mockFile.Name())
		mockStore.On("OpenForWrite", "/docs/test.txt").Return(mockFile, nil)

		// Execute
		item, err := svc.Upload(context.Background(), destination, filename, "rename", reader)

		// Assert
		assert.NoError(t, err)
		assert.Equal(t, "test.txt", item.Name)
		assert.Equal(t, "/docs/test.txt", item.Path)
		assert.Equal(t, "text/plain; charset=utf-8", item.MimeType)

		mockStore.AssertExpectations(t)
	})

	t.Run("fail on disallowed mime type", func(t *testing.T) {
		mockStore := new(storage.MockStorage)
		mockBus := event.NewBus()
		svc := NewFileService(mockStore, allowedMIMEs, "/tmp/thumbnails", mockBus)

		// Inputs
		filename := "evil.exe"
		destination := "/"
		// EXE signature (MZ...)
		content := "MZ\x90\x00\x03\x00\x00\x00"
		reader := strings.NewReader(content)

		// Mock expectations
		mockStore.On("MkdirAll", "/", fs.FileMode(0o755)).Return(nil)

		// Note: The service might call Stat before reading content to check conflicts
		mockStore.On("Stat", "/evil.exe").Return(nil, os.ErrNotExist)

		// Execute
		_, err := svc.Upload(context.Background(), destination, filename, "rename", reader)

		// Assert
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "UNSUPPORTED_TYPE")

		// Verify OpenForWrite was NEVER called
		mockStore.AssertNotCalled(t, "OpenForWrite", mock.Anything)
	})

	t.Run("handle conflict with skip policy", func(t *testing.T) {
		mockStore := new(storage.MockStorage)
		mockBus := event.NewBus()
		svc := NewFileService(mockStore, allowedMIMEs, "/tmp/thumbnails", mockBus)

		// Inputs
		filename := "exists.txt"
		destination := "/"
		reader := strings.NewReader("content")

		// Mock expectations
		mockStore.On("MkdirAll", "/", fs.FileMode(0o755)).Return(nil)

		// Simulate file exists (Stat returns info and nil error)
		mockInfo := &mockFileInfo{name: "exists.txt", size: 100}
		mockStore.On("Stat", "/exists.txt").Return(mockInfo, nil)

		// Execute
		_, err := svc.Upload(context.Background(), destination, filename, "skip", reader)

		// Assert
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "conflict_policy=skip")

		mockStore.AssertNotCalled(t, "OpenForWrite", mock.Anything)
	})
}

// mockFileInfo implements fs.FileInfo
type mockFileInfo struct {
	name string
	size int64
}

func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Mode() fs.FileMode  { return 0644 }
func (m *mockFileInfo) ModTime() time.Time { return time.Now() }
func (m *mockFileInfo) IsDir() bool        { return false }
func (m *mockFileInfo) Sys() any           { return nil }
