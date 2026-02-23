package storage

import (
	"io/fs"
	"os"

	"github.com/stretchr/testify/mock"
)

type MockStorage struct {
	mock.Mock
}

func (m *MockStorage) RootAbs() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockStorage) Resolve(clientPath string) (string, error) {
	args := m.Called(clientPath)
	return args.String(0), args.Error(1)
}

func (m *MockStorage) MkdirAll(clientPath string, perm fs.FileMode) error {
	args := m.Called(clientPath, perm)
	return args.Error(0)
}

func (m *MockStorage) Stat(clientPath string) (fs.FileInfo, error) {
	args := m.Called(clientPath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(fs.FileInfo), args.Error(1)
}

func (m *MockStorage) ReadDir(clientPath string) ([]fs.DirEntry, error) {
	args := m.Called(clientPath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]fs.DirEntry), args.Error(1)
}

func (m *MockStorage) RemoveAll(clientPath string) error {
	args := m.Called(clientPath)
	return args.Error(0)
}

func (m *MockStorage) Rename(oldPath string, newPath string) error {
	args := m.Called(oldPath, newPath)
	return args.Error(0)
}

func (m *MockStorage) OpenForRead(clientPath string) (*os.File, error) {
	args := m.Called(clientPath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*os.File), args.Error(1)
}

func (m *MockStorage) OpenForWrite(clientPath string) (*os.File, error) {
	args := m.Called(clientPath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*os.File), args.Error(1)
}
