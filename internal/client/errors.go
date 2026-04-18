package client

import (
	"errors"
	"fmt"
)

// APIError represents a structured error response from the Kargo API
// using the Connect protocol error format.
type APIError struct {
	HTTPStatus int
	Code       string
	Method     string
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("RPC %s returned %s (HTTP %d): %s", e.Method, e.Code, e.HTTPStatus, e.Message)
}

// IsNotFound reports whether err is an API error with code "not_found".
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == "not_found"
}

// IsAlreadyExists reports whether err is an API error with code "already_exists".
func IsAlreadyExists(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Code == "already_exists"
}
