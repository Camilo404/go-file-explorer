package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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

	"go-file-explorer/internal/event"
	"go-file-explorer/internal/model"
	"go-file-explorer/internal/storage"
	"go-file-explorer/internal/util"
	"go-file-explorer/pkg/apierror"

	"github.com/google/uuid"
)

type FileService struct {
	store            storage.Storage
	allowedMIMETypes map[string]struct{}
	thumbnailRoot    string
	bus              event.Bus
}

func NewFileService(store storage.Storage, allowedMIMETypes []string, thumbnailRoot string, bus event.Bus) *FileService {
	allowed := make(map[string]struct{}, len(allowedMIMETypes))
	for _, mimeType := range allowedMIMETypes {
		trimmed := strings.TrimSpace(strings.ToLower(mimeType))
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}

	if strings.TrimSpace(thumbnailRoot) == "" {
		thumbnailRoot = "./data/.thumbnails"
	}

	return &FileService{store: store, allowedMIMETypes: allowed, thumbnailRoot: thumbnailRoot, bus: bus}
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

	if s.bus != nil {
		s.bus.Publish(event.Event{
			ID:        uuid.NewString(),
			Type:      event.TypeFileUploaded,
			Payload:   item,
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		})
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

	// Detect whether the file is a video or an image.
	file, err := os.Open(resolved)
	if err != nil {
		return nil, nil, err
	}

	mimeType, err := util.DetectMIMEFromFile(file)
	_ = file.Close()
	if err != nil {
		return nil, nil, err
	}

	ext := strings.ToLower(filepath.Ext(resolved))
	isVideo := util.IsVideoMIME(mimeType)
	if !isVideo && strings.EqualFold(mimeType, "application/octet-stream") {
		isVideo = util.IsVideoExtension(ext)
	}

	if isVideo {
		return s.generateVideoThumbnail(resolved, thumbPath, size, info)
	}

	return s.generateImageThumbnail(resolved, thumbPath, size, info)
}

// generateImageThumbnail decodes an image, scales it, and writes a JPEG thumbnail.
func (s *FileService) generateImageThumbnail(resolved, thumbPath string, size int, info os.FileInfo) (*os.File, os.FileInfo, error) {
	file, err := os.Open(resolved)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	src, _, err := image.Decode(file)
	if err != nil {
		return nil, nil, apierror.New("UNSUPPORTED_TYPE", "cannot decode image", err.Error(), http.StatusUnsupportedMediaType)
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return nil, nil, apierror.New("UNSUPPORTED_TYPE", "invalid image dimensions", resolved, http.StatusUnsupportedMediaType)
	}

	return s.scaleAndSaveThumbnail(src, bounds, thumbPath, size, info)
}

// generateVideoThumbnail extracts a frame from a video using ffmpeg and saves
// it as a scaled JPEG thumbnail. If ffmpeg is not installed the endpoint returns
// UNSUPPORTED_TYPE so the client can fall back gracefully.
func (s *FileService) generateVideoThumbnail(resolved, thumbPath string, size int, info os.FileInfo) (*os.File, os.FileInfo, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, nil, apierror.New("UNSUPPORTED_TYPE", "ffmpeg not available for video thumbnails", "", http.StatusUnsupportedMediaType)
	}

	// Extract a single frame at ~1 s into the video (or 0 s if it's shorter).
	// Output raw JPEG to a temp file so we can decode → scale → save like images.
	tmpFile, err := os.CreateTemp(s.thumbnailRoot, "vtmp-*.jpg")
	if err != nil {
		return nil, nil, err
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	sizeStr := strconv.Itoa(size)

	//nolint:gosec // resolved path is already validated by storage layer
	cmd := exec.Command(
		ffmpegPath,
		"-hide_banner", "-loglevel", "error",
		"-ss", "1", // seek to 1 s (fast input seeking)
		"-i", resolved, // input file
		"-frames:v", "1", // extract one frame
		"-vf", "scale='min("+sizeStr+"\\,iw)':'min("+sizeStr+"\\,ih)':force_original_aspect_ratio=decrease",
		"-q:v", "2", // JPEG quality (2 = high)
		"-y", // overwrite
		tmpPath,
	)
	cmd.Stderr = nil
	cmd.Stdout = nil

	if err := cmd.Run(); err != nil {
		return nil, nil, apierror.New("UNSUPPORTED_TYPE", "failed to extract video frame", err.Error(), http.StatusUnsupportedMediaType)
	}

	// ffmpeg already scaled the frame; just copy it to the final thumbnail path.
	src, err := os.Open(tmpPath)
	if err != nil {
		return nil, nil, err
	}
	defer src.Close()

	dst, err := os.OpenFile(thumbPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, nil, err
	}

	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return nil, nil, err
	}
	_ = dst.Close()

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

// scaleAndSaveThumbnail scales a decoded image to the given size and saves it
// as a JPEG file at thumbPath. It returns the opened thumbnail file and its info.
func (s *FileService) scaleAndSaveThumbnail(src image.Image, bounds image.Rectangle, thumbPath string, size int, info os.FileInfo) (*os.File, os.FileInfo, error) {
	width := bounds.Dx()
	height := bounds.Dy()

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

	encodeErr := jpeg.Encode(thumbWriter, dst, &jpeg.Options{Quality: 95})
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
			isImage := util.IsImageMIME(mimeType)
			if !isImage && strings.EqualFold(mimeType, "application/octet-stream") {
				isImage = util.IsImageExtension(item.Extension)
			}

			isVideo := util.IsVideoMIME(mimeType)
			if !isVideo && strings.EqualFold(mimeType, "application/octet-stream") {
				isVideo = util.IsVideoExtension(item.Extension)
			}

			if isImage {
				item.IsImage = true
				item.PreviewURL = "/api/v1/files/preview?path=" + url.QueryEscape(item.Path)
				thumbnailSupported := util.IsThumbnailMIME(mimeType)
				if !thumbnailSupported && strings.EqualFold(mimeType, "application/octet-stream") {
					thumbnailSupported = util.IsThumbnailExtension(item.Extension)
				}

				if thumbnailSupported {
					item.ThumbnailURL = "/api/v1/files/thumbnail?path=" + url.QueryEscape(item.Path) + "&size=256"
				}
			} else if isVideo {
				item.IsVideo = true
				item.PreviewURL = "/api/v1/files/preview?path=" + url.QueryEscape(item.Path)
				item.ThumbnailURL = "/api/v1/files/thumbnail?path=" + url.QueryEscape(item.Path) + "&size=256"
			}
		}
	}

	return item, nil
}

func (s *FileService) isAllowedMIME(mimeType string) bool {
	if len(s.allowedMIMETypes) == 0 {
		return true
	}

	baseMIME, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		// If we can't parse it, try the raw string as fallback
		baseMIME = strings.ToLower(strings.TrimSpace(mimeType))
	}

	_, exact := s.allowedMIMETypes[baseMIME]
	if exact {
		return true
	}

	for allowed := range s.allowedMIMETypes {
		if strings.HasSuffix(allowed, "/*") {
			prefix := strings.TrimSuffix(allowed, "*")
			if strings.HasPrefix(baseMIME, prefix) {
				return true
			}
		}
	}

	return false
}
