package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type Storage struct {
	validator *PathValidator
}

func (s *Storage) RootAbs() string {
	return s.validator.RootAbs()
}

func New(root string) (*Storage, error) {
	validator, err := NewPathValidator(root)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(validator.RootAbs(), 0o755); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}

	return &Storage{validator: validator}, nil
}

func (s *Storage) Resolve(clientPath string) (string, error) {
	return s.validator.ResolvePath(clientPath)
}

func (s *Storage) MkdirAll(clientPath string, perm fs.FileMode) error {
	resolved, err := s.Resolve(clientPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(resolved, perm); err != nil {
		return fmt.Errorf("mkdir %q: %w", clientPath, err)
	}

	return nil
}

func (s *Storage) Stat(clientPath string) (fs.FileInfo, error) {
	resolved, err := s.Resolve(clientPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return nil, err
	}

	return info, nil
}

func (s *Storage) ReadDir(clientPath string) ([]fs.DirEntry, error) {
	resolved, err := s.Resolve(clientPath)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, err
	}

	return entries, nil
}

func (s *Storage) RemoveAll(clientPath string) error {
	resolved, err := s.Resolve(clientPath)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(resolved); err != nil {
		return fmt.Errorf("remove %q: %w", clientPath, err)
	}

	return nil
}

func (s *Storage) Rename(oldPath string, newPath string) error {
	oldResolved, err := s.Resolve(oldPath)
	if err != nil {
		return err
	}

	newResolved, err := s.Resolve(newPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(newResolved), 0o755); err != nil {
		return fmt.Errorf("prepare destination %q: %w", newPath, err)
	}

	if err := os.Rename(oldResolved, newResolved); err != nil {
		return fmt.Errorf("rename %q to %q: %w", oldPath, newPath, err)
	}

	return nil
}

func (s *Storage) OpenForRead(clientPath string) (*os.File, error) {
	resolved, err := s.Resolve(clientPath)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(resolved)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (s *Storage) OpenForWrite(clientPath string) (*os.File, error) {
	resolved, err := s.Resolve(clientPath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return nil, fmt.Errorf("create parent directory: %w", err)
	}

	file, err := os.OpenFile(resolved, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}

	return file, nil
}
