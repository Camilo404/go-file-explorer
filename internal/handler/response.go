package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"

	"go-file-explorer/internal/model"
	"go-file-explorer/pkg/apierror"
)

func writeSuccess(w http.ResponseWriter, status int, data any, meta *model.Meta) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(model.APIResponse{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	body := &model.APIError{
		Code:    "INTERNAL_ERROR",
		Message: "Unexpected server error",
	}

	var apiErr *apierror.APIError
	if errors.As(err, &apiErr) {
		status = apiErr.HTTPStatus
		body.Code = apiErr.Code
		body.Message = apiErr.Message
		body.Details = apiErr.Details
	} else if errors.Is(err, model.ErrUserNotFound) {
		status = http.StatusNotFound
		body.Code = "NOT_FOUND"
		body.Message = "User not found"
	} else if errors.Is(err, model.ErrUserAlreadyExists) {
		status = http.StatusConflict
		body.Code = "ALREADY_EXISTS"
		body.Message = "User already exists"
	} else if errors.Is(err, model.ErrInvalidCredentials) {
		status = http.StatusUnauthorized
		body.Code = "UNAUTHORIZED"
		body.Message = "Invalid credentials"
	} else if errors.Is(err, model.ErrUnauthorized) {
		status = http.StatusUnauthorized
		body.Code = "UNAUTHORIZED"
		body.Message = "Authentication required"
	} else if errors.Is(err, model.ErrForbidden) {
		status = http.StatusForbidden
		body.Code = "FORBIDDEN"
		body.Message = "Access denied"
	} else if errors.Is(err, model.ErrTokenNotFound) || errors.Is(err, model.ErrTokenExpired) {
		status = http.StatusUnauthorized
		body.Code = "UNAUTHORIZED"
		body.Message = "Invalid or expired token"
	} else if errors.Is(err, model.ErrFileNotFound) {
		status = http.StatusNotFound
		body.Code = "NOT_FOUND"
		body.Message = "File not found"
	} else if errors.Is(err, model.ErrDirectoryNotFound) {
		status = http.StatusNotFound
		body.Code = "NOT_FOUND"
		body.Message = "Directory not found"
	} else if errors.Is(err, model.ErrPathConflict) {
		status = http.StatusConflict
		body.Code = "CONFLICT"
		body.Message = "Path already exists"
	} else if errors.Is(err, model.ErrJobNotFound) {
		status = http.StatusNotFound
		body.Code = "NOT_FOUND"
		body.Message = "Job not found"
	} else if errors.Is(err, model.ErrShareNotFound) {
		status = http.StatusNotFound
		body.Code = "NOT_FOUND"
		body.Message = "Share not found"
	} else if errors.Is(err, model.ErrShareExpired) {
		status = http.StatusGone
		body.Code = "GONE"
		body.Message = "Share expired"
	} else if errors.Is(err, model.ErrTrashItemNotFound) {
		status = http.StatusNotFound
		body.Code = "NOT_FOUND"
		body.Message = "Trash item not found"
	} else if errors.Is(err, model.ErrItemAlreadyRestored) {
		status = http.StatusConflict
		body.Code = "CONFLICT"
		body.Message = "Item already restored"
	} else if errors.Is(err, model.ErrInvalidInput) {
		status = http.StatusBadRequest
		body.Code = "BAD_REQUEST"
		body.Message = "Invalid input"
	} else if errors.Is(err, os.ErrPermission) {
		status = http.StatusForbidden
		body.Code = "PERMISSION_DENIED"
		body.Message = "Permission denied on the filesystem"
		body.Details = err.Error()
	} else if errors.Is(err, os.ErrNotExist) {
		status = http.StatusNotFound
		body.Code = "NOT_FOUND"
		body.Message = "Path not found"
		body.Details = err.Error()
	} else if errors.Is(err, os.ErrExist) {
		status = http.StatusConflict
		body.Code = "ALREADY_EXISTS"
		body.Message = "Path already exists"
		body.Details = err.Error()
	} else {
		// Log unclassified errors so they are visible in container logs.
		slog.Error("unhandled error in writeError", "error", err.Error())
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(model.APIResponse{
		Success: false,
		Error:   body,
	})
}
