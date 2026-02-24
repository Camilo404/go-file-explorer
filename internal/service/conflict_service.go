package service

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go-file-explorer/internal/storage"
	"go-file-explorer/pkg/apierror"
)

const (
	ConflictPolicyOverwrite = "overwrite"
	ConflictPolicyRename    = "rename"
	ConflictPolicySkip      = "skip"
)

func normalizeConflictPolicy(raw string) (string, error) {
	policy := strings.ToLower(strings.TrimSpace(raw))
	if policy == "" {
		policy = ConflictPolicyRename
	}

	switch policy {
	case ConflictPolicyOverwrite, ConflictPolicyRename, ConflictPolicySkip:
		return policy, nil
	default:
		return "", apierror.New("BAD_REQUEST", "invalid conflict_policy (allowed: overwrite|rename|skip)", raw, http.StatusBadRequest)
	}
}

// statNotFound returns true when the error from store.Stat indicates the path
// does not exist. It checks both the classified apierror wrapper (HTTP 404)
// that storage.Local produces and the raw os-level error as a fallback.
func statNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *apierror.APIError
	if errors.As(err, &apiErr) {
		return apiErr.HTTPStatus == http.StatusNotFound
	}
	// Fallback: in case a storage implementation returns a raw os error.
	return errors.Is(err, os.ErrNotExist)
}

func resolveConflictTarget(store storage.Storage, desiredPath string, policy string) (string, bool, error) {
	normalizedPolicy, err := normalizeConflictPolicy(policy)
	if err != nil {
		return "", false, err
	}

	// Check whether the target path already exists.
	_, statErr := store.Stat(desiredPath)
	if statErr != nil {
		// Path does not exist – no conflict regardless of policy.
		if statNotFound(statErr) {
			return desiredPath, false, nil
		}
		// Unexpected error (e.g. permission denied) – propagate it.
		return "", false, statErr
	}

	switch normalizedPolicy {
	case ConflictPolicySkip:
		return "", true, nil
	case ConflictPolicyOverwrite:
		// We use RemoveAll which internally resolves, but that's fine as it's part of the interface
		if removeErr := store.RemoveAll(desiredPath); removeErr != nil {
			return "", false, fmt.Errorf("overwrite target %q: %w", desiredPath, removeErr)
		}
		return desiredPath, false, nil
	case ConflictPolicyRename:
		candidate := desiredPath
		ext := filepath.Ext(candidate)
		baseName := strings.TrimSuffix(filepath.Base(candidate), ext)
		parent := filepath.Dir(candidate)

		for index := 1; index <= 10000; index++ {
			nextName := fmt.Sprintf("%s (%d)%s", baseName, index, ext)
			nextPath := normalizeAPIPath(filepath.Join(parent, nextName))

			_, nextStatErr := store.Stat(nextPath)
			if nextStatErr != nil {
				if statNotFound(nextStatErr) {
					return nextPath, false, nil
				}
				return "", false, nextStatErr
			}
		}

		return "", false, apierror.New("CONFLICT", "could not resolve unique target name", desiredPath, http.StatusConflict)
	default:
		return "", false, apierror.New("BAD_REQUEST", "invalid conflict policy", normalizedPolicy, http.StatusBadRequest)
	}
}
