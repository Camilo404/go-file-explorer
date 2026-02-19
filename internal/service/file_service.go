package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "image/gif"
	"image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"go-file-explorer/internal/model"
	"go-file-explorer/internal/storage"
	"go-file-explorer/internal/util"
	"go-file-explorer/pkg/apierror"
)

type FileService struct {
	store            *storage.Storage
	allowedMIMETypes map[string]struct{}
	thumbnailRoot    string
}

func NewFileService(store *storage.Storage, allowedMIMETypes []string, thumbnailRoot string) *FileService {
	allowed := make(map[string]struct{}, len(allowedMIMETypes))
	for _, mimeType := range allowedMIMETypes {
		trimmed := strings.TrimSpace(strings.ToLower(mimeType))
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}

	if strings.TrimSpace(thumbnailRoot) == "" {
		thumbnailRoot = "./state/thumbnails"
	}

	return &FileService{store: store, allowedMIMETypes: allowed, thumbnailRoot: thumbnailRoot}
}

func (s *FileService) Upload(_ context.Context, destination string, filename string, conflictPolicy string, reader io.Reader) (model.UploadItem, error) {
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
	targetPath, skipped, err := resolveConflictTarget(s.store, targetPath, conflictPolicy)
	if err != nil {
		return model.UploadItem{}, err
	}
	if skipped {
		return model.UploadItem{}, apierror.New("CONFLICT", "target already exists and conflict_policy=skip", safeName, http.StatusConflict)
	}

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

func (s *FileService) GetThumbnail(path string, size int) (*os.File, os.FileInfo, error) {
	if size <= 0 {
		size = 256
	}

	resolved, err := s.store.Resolve(path)
	if err != nil {
		return nil, nil, err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, apierror.New("NOT_FOUND", "file not found", path, http.StatusNotFound)
		}
		return nil, nil, err
	}

	if info.IsDir() {
		return nil, nil, apierror.New("BAD_REQUEST", "path points to a directory", path, http.StatusBadRequest)
	}

	file, err := os.Open(resolved)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	mimeType, err := util.DetectMIMEFromFile(file)
	if err != nil {
		return nil, nil, err
	}

	if !util.IsThumbnailMIME(mimeType) {
		return nil, nil, apierror.New("UNSUPPORTED_TYPE", "thumbnail not supported for this MIME type", mimeType, http.StatusUnsupportedMediaType)
	}

	if err := os.MkdirAll(s.thumbnailRoot, 0o755); err != nil {
		return nil, nil, err
	}

	thumbPath := s.thumbnailPath(resolved, size)
	if thumbInfo, err := os.Stat(thumbPath); err == nil {
		if !thumbInfo.ModTime().Before(info.ModTime()) {
			thumbFile, openErr := os.Open(thumbPath)
			if openErr == nil {
				return thumbFile, thumbInfo, nil
			}
		}
	}

	if _, err := file.Seek(0, 0); err != nil {
		return nil, nil, err
	}

	src, _, err := image.Decode(file)
	if err != nil {
		return nil, nil, apierror.New("UNSUPPORTED_TYPE", "cannot decode image", err.Error(), http.StatusUnsupportedMediaType)
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, nil, apierror.New("UNSUPPORTED_TYPE", "invalid image dimensions", path, http.StatusUnsupportedMediaType)
	}

	maxDim := width
	if height > maxDim {
		maxDim = height
	}

	scale := float64(size) / float64(maxDim)
	if scale > 1 {
		scale = 1
	}

	targetWidth := int(math.Round(float64(width) * scale))
	targetHeight := int(math.Round(float64(height) * scale))
	if targetWidth < 1 {
		targetWidth = 1
	}
	if targetHeight < 1 {
		targetHeight = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	thumbWriter, err := os.OpenFile(thumbPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, err
	}

	encodeErr := jpeg.Encode(thumbWriter, dst, &jpeg.Options{Quality: 80})
	closeErr := thumbWriter.Close()
	if encodeErr != nil {
		return nil, nil, encodeErr
	}
	if closeErr != nil {
		return nil, nil, closeErr
	}

	_ = os.Chtimes(thumbPath, time.Now().UTC(), info.ModTime())

	thumbFile, err := os.Open(thumbPath)
	if err != nil {
		return nil, nil, err
	}

	thumbInfo, err := os.Stat(thumbPath)
	if err != nil {
		_ = thumbFile.Close()
		return nil, nil, err
	}

	return thumbFile, thumbInfo, nil
}

func (s *FileService) thumbnailPath(resolvedPath string, size int) string {
	hash := sha256.Sum256([]byte(resolvedPath + "|" + strconv.Itoa(size)))
	name := hex.EncodeToString(hash[:]) + ".jpg"
	return filepath.Join(s.thumbnailRoot, name)
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
			if util.IsImageMIME(mimeType) {
				item.IsImage = true
				item.PreviewURL = "/api/v1/files/preview?path=" + url.QueryEscape(item.Path)
				if util.IsThumbnailMIME(mimeType) {
					item.ThumbnailURL = "/api/v1/files/thumbnail?path=" + url.QueryEscape(item.Path) + "&size=256"
				}
			}
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
