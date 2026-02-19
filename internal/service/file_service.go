package service

import (
	"bytes"
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

type FileService struct {
	store            *storage.Storage
	allowedMIMETypes map[string]struct{}
}

func NewFileService(store *storage.Storage, allowedMIMETypes []string) *FileService {
	allowed := make(map[string]struct{}, len(allowedMIMETypes))
	for _, mimeType := range allowedMIMETypes {
		trimmed := strings.TrimSpace(strings.ToLower(mimeType))
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}

	return &FileService{store: store, allowedMIMETypes: allowed}
}

func (s *FileService) Upload(_ context.Context, destination string, filename string, reader io.Reader) (model.UploadItem, error) {
	safeName, err := util.SanitizeFilename(filename, false)
	if err != nil {
		return model.UploadItem{}, err
	}

	if strings.TrimSpace(destination) == "" {
		destination = "/"
	}

	destinationPath := normalizeAPIPath(destination)
	if err := s.store.MkdirAll(destinationPath, 0o755); err != nil {
		return model.UploadItem{}, err
	}

	targetPath := normalizeAPIPath(filepath.Join(destinationPath, safeName))

	sniffBuffer := make([]byte, 512)
	n, readErr := io.ReadFull(reader, sniffBuffer)
	if readErr != nil && readErr != io.ErrUnexpectedEOF && readErr != io.EOF {
		return model.UploadItem{}, readErr
	}

	detectedMIME := http.DetectContentType(sniffBuffer[:n])
	if !s.isAllowedMIME(detectedMIME) {
		return model.UploadItem{}, apierror.New("UNSUPPORTED_TYPE", "file MIME type is not allowed", detectedMIME, http.StatusUnsupportedMediaType)
	}

	writer, err := s.store.OpenForWrite(targetPath)
	if err != nil {
		return model.UploadItem{}, err
	}
	defer writer.Close()

	contentReader := io.MultiReader(bytes.NewReader(sniffBuffer[:n]), reader)
	written, err := io.CopyBuffer(writer, contentReader, make([]byte, 32*1024))
	if err != nil {
		return model.UploadItem{}, err
	}

	item := model.UploadItem{
		Name:     safeName,
		Path:     targetPath,
		Size:     written,
		MimeType: detectedMIME,
	}

	return item, nil
}

func (s *FileService) GetFile(path string) (*os.File, os.FileInfo, string, error) {
	resolved, err := s.store.Resolve(path)
	if err != nil {
		return nil, nil, "", err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, "", apierror.New("NOT_FOUND", "file not found", path, http.StatusNotFound)
		}
		return nil, nil, "", err
	}

	if info.IsDir() {
		return nil, nil, "", apierror.New("BAD_REQUEST", "path points to a directory", path, http.StatusBadRequest)
	}

	file, err := os.Open(resolved)
	if err != nil {
		return nil, nil, "", err
	}

	mimeType, err := util.DetectMIMEFromFile(file)
	if err != nil {
		_ = file.Close()
		return nil, nil, "", err
	}

	return file, info, mimeType, nil
}

func (s *FileService) GetDirectoryForArchive(path string) (string, string, error) {
	resolved, err := s.store.Resolve(path)
	if err != nil {
		return "", "", err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", apierror.New("NOT_FOUND", "directory not found", path, http.StatusNotFound)
		}
		return "", "", err
	}

	if !info.IsDir() {
		return "", "", apierror.New("BAD_REQUEST", "archive download requires a directory path", path, http.StatusBadRequest)
	}

	name := strings.TrimSpace(filepath.Base(resolved))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "archive"
	}

	return resolved, name, nil
}

func (s *FileService) GetInfo(path string) (model.FileItem, error) {
	resolved, err := s.store.Resolve(path)
	if err != nil {
		return model.FileItem{}, err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return model.FileItem{}, apierror.New("NOT_FOUND", "path not found", path, http.StatusNotFound)
		}
		return model.FileItem{}, err
	}

	item := model.FileItem{
		Name:        info.Name(),
		Path:        toAPIPath(resolved, s.store.RootAbs()),
		Permissions: info.Mode().String(),
		ModifiedAt:  info.ModTime().UTC(),
		CreatedAt:   info.ModTime().UTC(),
	}

	if info.IsDir() {
		item.Type = "directory"
		item.Size = 0
		children, readErr := os.ReadDir(resolved)
		if readErr == nil {
			count := len(children)
			item.ItemCount = &count
		}
		return item, nil
	}

	item.Type = "file"
	item.Size = info.Size()
	item.SizeHuman = humanizeSize(info.Size())
	item.Extension = strings.ToLower(filepath.Ext(info.Name()))

	file, openErr := os.Open(resolved)
	if openErr == nil {
		defer file.Close()
		mimeType, detectErr := util.DetectMIMEFromFile(file)
		if detectErr == nil {
			item.MimeType = mimeType
		}
	}

	return item, nil
}

func (s *FileService) isAllowedMIME(mimeType string) bool {
	if len(s.allowedMIMETypes) == 0 {
		return true
	}

	_, exact := s.allowedMIMETypes[strings.ToLower(strings.TrimSpace(mimeType))]
	if exact {
		return true
	}

	for allowed := range s.allowedMIMETypes {
		if strings.HasSuffix(allowed, "/*") {
			prefix := strings.TrimSuffix(allowed, "*")
			if strings.HasPrefix(strings.ToLower(mimeType), prefix) {
				return true
			}
		}
	}

	return false
}
