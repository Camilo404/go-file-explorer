package apierror

import "fmt"

type APIError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
	HTTPStatus int    `json:"-"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}

	if e.Details != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Details)
	}

	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func New(code string, message string, details string, status int) *APIError {
	return &APIError{Code: code, Message: message, Details: details, HTTPStatus: status}
}
