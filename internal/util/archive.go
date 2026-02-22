package util

import (
	"archive/zip"
	"fmt"
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

// Compress creates a zip file from multiple source paths.
func Compress(sources []string, destZip string) error {
	zipFile, err := os.Create(destZip)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, source := range sources {
		info, err := os.Stat(source)
		if err != nil {
			return err
		}

		if info.IsDir() {
			baseDir := filepath.Dir(source)
			err = filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				relPath, err := filepath.Rel(baseDir, path)
				if err != nil {
					return err
				}

				zipPath := filepath.ToSlash(relPath)
				if d.IsDir() {
					if !strings.HasSuffix(zipPath, "/") {
						zipPath += "/"
					}
					_, err := zipWriter.Create(zipPath)
					return err
				}

				fileToZip, err := os.Open(path)
				if err != nil {
					return err
				}
				defer fileToZip.Close()

				w, err := zipWriter.Create(zipPath)
				if err != nil {
					return err
				}

				_, err = io.Copy(w, fileToZip)
				return err
			})
			if err != nil {
				return err
			}
		} else {
			relPath := filepath.Base(source)
			fileToZip, err := os.Open(source)
			if err != nil {
				return err
			}
			
			w, err := zipWriter.Create(relPath)
			if err != nil {
				fileToZip.Close()
				return err
			}

			_, err = io.Copy(w, fileToZip)
			fileToZip.Close()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// CheckZipConflicts checks if extracting the zip file would overwrite any existing files.
func CheckZipConflicts(srcZip string, destDir string) ([]string, error) {
	var conflicts []string

	r, err := zip.OpenReader(srcZip)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)

		// Check for Zip Slip
		if !strings.HasPrefix(fpath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return nil, fmt.Errorf("illegal file path: %s", fpath)
		}

		if _, err := os.Stat(fpath); err == nil {
			conflicts = append(conflicts, f.Name)
		}
	}
	return conflicts, nil
}

// Decompress extracts a zip file to a destination directory.
func Decompress(srcZip string, destDir string) ([]string, error) {
	var extractedFiles []string

	r, err := zip.OpenReader(srcZip)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)

		// Check for Zip Slip
		if !strings.HasPrefix(fpath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return nil, fmt.Errorf("illegal file path: %s", fpath)
		}

		extractedFiles = append(extractedFiles, f.Name)

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return nil, err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return nil, err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return nil, err
		}

		_, err = io.Copy(outFile, rc)

		outFile.Close()
		rc.Close()

		if err != nil {
			return nil, err
		}
	}
	return extractedFiles, nil
}
