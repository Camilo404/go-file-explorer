package handler

import (
	"encoding/json"
	"errors"
	"net/http"

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
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(model.APIResponse{
		Success: false,
		Error:   body,
	})
}
