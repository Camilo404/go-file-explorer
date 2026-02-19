package service

import (
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

func resolveConflictTarget(store *storage.Storage, desiredPath string, policy string) (string, bool, error) {
	normalizedPolicy, err := normalizeConflictPolicy(policy)
	if err != nil {
		return "", false, err
	}

	desiredResolved, err := store.Resolve(desiredPath)
	if err != nil {
		return "", false, err
	}

	if _, statErr := os.Stat(desiredResolved); os.IsNotExist(statErr) {
		return desiredPath, false, nil
	}

	switch normalizedPolicy {
	case ConflictPolicySkip:
		return "", true, nil
	case ConflictPolicyOverwrite:
		if removeErr := os.RemoveAll(desiredResolved); removeErr != nil {
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
			nextResolved, resolveErr := store.Resolve(nextPath)
			if resolveErr != nil {
				return "", false, resolveErr
			}
			if _, statErr := os.Stat(nextResolved); os.IsNotExist(statErr) {
				return nextPath, false, nil
			}
		}

		return "", false, apierror.New("CONFLICT", "could not resolve unique target name", desiredPath, http.StatusConflict)
	default:
		return "", false, apierror.New("BAD_REQUEST", "invalid conflict policy", normalizedPolicy, http.StatusBadRequest)
	}
}
