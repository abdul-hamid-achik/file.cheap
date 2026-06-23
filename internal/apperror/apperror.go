// Package apperror provides typed error types for fcheap boundary handling.
package apperror

import "errors"

// Error is a typed error with a machine-readable code and optional wrapped cause.
type Error struct {
	Code     string
	Message  string
	Internal error
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Internal
}

var (
	ErrStashNotFound = &Error{
		Code:    "stash_not_found",
		Message: "Stash not found",
	}
	ErrStashExists = &Error{
		Code:    "stash_exists",
		Message: "A stash with this ID already exists",
	}
	ErrFileNotFound = &Error{
		Code:    "file_not_found",
		Message: "File not found",
	}
	ErrInvalidPath = &Error{
		Code:    "invalid_path",
		Message: "Invalid file path",
	}
	ErrArchiveFailed = &Error{
		Code:    "archive_failed",
		Message: "Failed to create or extract archive",
	}
	ErrRestoreFailed = &Error{
		Code:    "restore_failed",
		Message: "Failed to restore stash",
	}
	ErrIndexFailed = &Error{
		Code:    "index_failed",
		Message: "Failed to index stash content",
	}
	ErrDependencyMissing = &Error{
		Code:    "dependency_missing",
		Message: "Required external dependency not found",
	}
	ErrInternal = &Error{
		Code:    "internal_error",
		Message: "An unexpected error occurred",
	}
)

// New creates a new typed error with the given code and message.
func New(code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// Wrap wraps an underlying error with an apperror type.
func Wrap(err error, appErr *Error) *Error {
	return &Error{
		Code:     appErr.Code,
		Message:  appErr.Message,
		Internal: err,
	}
}

// WrapWithMessage creates a new error wrapping an underlying error with a custom message.
func WrapWithMessage(err error, code, message string) *Error {
	return &Error{
		Code:     code,
		Message:  message,
		Internal: err,
	}
}

// Is reports whether err matches the target error code.
func Is(err error, target *Error) bool {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code == target.Code
	}
	return false
}

// Code extracts the error code from an error, defaulting to internal_error.
func Code(err error) string {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ErrInternal.Code
}
