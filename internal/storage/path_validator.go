package storage

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"unicode"

	"go-file-explorer/pkg/apierror"
)

type PathValidator struct {
	rootAbs string
}

func NewPathValidator(root string) (*PathValidator, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("root path cannot be empty")
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve storage root: %w", err)
	}

	return &PathValidator{rootAbs: rootAbs}, nil
}

func (v *PathValidator) RootAbs() string {
	return v.rootAbs
}

func (v *PathValidator) ResolvePath(clientPath string) (string, error) {
	normalized := strings.ReplaceAll(strings.TrimSpace(clientPath), `\`, "/")
	if normalized == "" || normalized == "/" {
		return v.rootAbs, nil
	}

	if strings.Contains(normalized, "\x00") || hasControlCharacters(normalized) {
		return "", apierror.New("INVALID_PATH", "path contains invalid characters", clientPath, http.StatusBadRequest)
	}

	segments := strings.Split(normalized, "/")
	for _, segment := range segments {
		if segment == ".." {
			return "", apierror.New("PATH_TRAVERSAL", "path traversal attempt detected", clientPath, http.StatusForbidden)
		}
	}

	cleanRel := filepath.Clean(strings.TrimPrefix(normalized, "/"))
	if cleanRel == "." {
		return v.rootAbs, nil
	}

	resolved := filepath.Join(v.rootAbs, cleanRel)
	resolvedAbs, err := filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}

	if !isWithinRoot(v.rootAbs, resolvedAbs) {
		return "", apierror.New("PATH_TRAVERSAL", "resolved path is outside storage root", clientPath, http.StatusForbidden)
	}

	return resolvedAbs, nil
}

func hasControlCharacters(value string) bool {
	for _, char := range value {
		if unicode.IsControl(char) {
			return true
		}
	}

	return false
}

func isWithinRoot(rootAbs string, candidateAbs string) bool {
	if candidateAbs == rootAbs {
		return true
	}

	rootWithSeparator := rootAbs + string(filepath.Separator)
	return strings.HasPrefix(candidateAbs, rootWithSeparator)
}
