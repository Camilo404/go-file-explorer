package util

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func StreamZipFromDirectory(rootDir string, writer io.Writer) error {
	zipWriter := zip.NewWriter(writer)
	defer zipWriter.Close()

	baseDir := filepath.Clean(rootDir)

	return filepath.WalkDir(baseDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if path == baseDir {
			return nil
		}

		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}

		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}

		zipPath := filepath.ToSlash(rel)
		if entry.IsDir() {
			if !strings.HasSuffix(zipPath, "/") {
				zipPath += "/"
			}
			_, err := zipWriter.Create(zipPath)
			return err
		}

		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()

		zipFile, err := zipWriter.Create(zipPath)
		if err != nil {
			return err
		}

		_, err = io.Copy(zipFile, source)
		return err
	})
}
