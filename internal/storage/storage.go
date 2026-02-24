package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"go-file-explorer/pkg/apierror"
)

func classifyOSError(err error, context string) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, os.ErrPermission) {
		return apierror.New("PERMISSION_DENIED", "permission denied", context, http.StatusForbidden)
	}
	if errors.Is(err, os.ErrNotExist) {
		return apierror.New("NOT_FOUND", "path not found", context, http.StatusNotFound)
	}
	if errors.Is(err, os.ErrExist) {
		return apierror.New("ALREADY_EXISTS", "path already exists", context, http.StatusConflict)
	}

	return err
}

type Storage interface {
	RootAbs() string
	Resolve(clientPath string) (string, error)
	MkdirAll(clientPath string, perm fs.FileMode) error
	Stat(clientPath string) (fs.FileInfo, error)
	ReadDir(clientPath string) ([]fs.DirEntry, error)
	RemoveAll(clientPath string) error
	Rename(oldPath string, newPath string) error
	OpenForRead(clientPath string) (*os.File, error)
	OpenForWrite(clientPath string) (*os.File, error)
}

type Local struct {
	validator *PathValidator
}

func (s *Local) RootAbs() string {
	return s.validator.RootAbs()
}

func New(root string) (Storage, error) {
	validator, err := NewPathValidator(root)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(validator.RootAbs(), 0o755); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}

	return &Local{validator: validator}, nil
}

func (s *Local) Resolve(clientPath string) (string, error) {
	return s.validator.ResolvePath(clientPath)
}

func (s *Local) MkdirAll(clientPath string, perm fs.FileMode) error {
	resolved, err := s.Resolve(clientPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(resolved, perm); err != nil {
		return classifyOSError(err, clientPath)
	}

	return nil
}

func (s *Local) Stat(clientPath string) (fs.FileInfo, error) {
	resolved, err := s.Resolve(clientPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return nil, classifyOSError(err, clientPath)
	}

	return info, nil
}

func (s *Local) ReadDir(clientPath string) ([]fs.DirEntry, error) {
	resolved, err := s.Resolve(clientPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, classifyOSError(err, clientPath)
	}

	return entries, nil
}

func (s *Local) RemoveAll(clientPath string) error {
	resolved, err := s.Resolve(clientPath)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(resolved); err != nil {
		return classifyOSError(err, clientPath)
	}

	return nil
}

func (s *Local) Rename(oldPath string, newPath string) error {
	oldResolved, err := s.Resolve(oldPath)
	if err != nil {
		return err
	}

	newResolved, err := s.Resolve(newPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(newResolved), 0o755); err != nil {
		return classifyOSError(err, newPath)
	}

	if err := os.Rename(oldResolved, newResolved); err != nil {
		return classifyOSError(err, fmt.Sprintf("%s -> %s", oldPath, newPath))
	}

	return nil
}

func (s *Local) OpenForRead(clientPath string) (*os.File, error) {
	resolved, err := s.Resolve(clientPath)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(resolved)
	if err != nil {
		return nil, classifyOSError(err, clientPath)
	}

	return file, nil
}

func (s *Local) OpenForWrite(clientPath string) (*os.File, error) {
	resolved, err := s.Resolve(clientPath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return nil, classifyOSError(err, clientPath)
	}

	file, err := os.OpenFile(resolved, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, classifyOSError(err, clientPath)
	}

	return file, nil
}
