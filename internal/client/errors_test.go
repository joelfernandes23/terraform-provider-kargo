package client

import (
	"errors"
	"fmt"
	"testing"
)

func TestAPIError_Error(t *testing.T) {
	err := &APIError{
		HTTPStatus: 404,
		Code:       "not_found",
		Method:     "GetProject",
		Message:    "project not found",
	}

	expected := "RPC GetProject returned not_found (HTTP 404): project not found"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestAPIError_ErrorsAs(t *testing.T) {
	orig := &APIError{
		HTTPStatus: 409,
		Code:       "already_exists",
		Method:     "CreateResource",
		Message:    "already exists",
	}
	wrapped := fmt.Errorf("outer: %w", orig)

	var apiErr *APIError
	if !errors.As(wrapped, &apiErr) {
		t.Fatal("expected errors.As to find *APIError")
	}
	if apiErr.HTTPStatus != 409 {
		t.Errorf("expected HTTPStatus 409, got %d", apiErr.HTTPStatus)
	}
	if apiErr.Code != "already_exists" {
		t.Errorf("expected Code %q, got %q", "already_exists", apiErr.Code)
	}
	if apiErr.Method != "CreateResource" {
		t.Errorf("expected Method %q, got %q", "CreateResource", apiErr.Method)
	}
	if apiErr.Message != "already exists" {
		t.Errorf("expected Message %q, got %q", "already exists", apiErr.Message)
	}
}

func TestIsNotFound_True(t *testing.T) {
	err := fmt.Errorf("wrap: %w", &APIError{Code: "not_found"})
	if !IsNotFound(err) {
		t.Error("expected IsNotFound to return true")
	}
}

func TestIsNotFound_False_DifferentCode(t *testing.T) {
	err := &APIError{Code: "already_exists"}
	if IsNotFound(err) {
		t.Error("expected IsNotFound to return false for already_exists")
	}
}

func TestIsNotFound_False_NonAPIError(t *testing.T) {
	err := fmt.Errorf("plain error")
	if IsNotFound(err) {
		t.Error("expected IsNotFound to return false for non-APIError")
	}
}

func TestIsNotFound_False_Nil(t *testing.T) {
	if IsNotFound(nil) {
		t.Error("expected IsNotFound to return false for nil")
	}
}

func TestIsAlreadyExists_True(t *testing.T) {
	err := fmt.Errorf("wrap: %w", &APIError{Code: "already_exists"})
	if !IsAlreadyExists(err) {
		t.Error("expected IsAlreadyExists to return true")
	}
}

func TestIsAlreadyExists_False(t *testing.T) {
	err := &APIError{Code: "not_found"}
	if IsAlreadyExists(err) {
		t.Error("expected IsAlreadyExists to return false for not_found")
	}
}
