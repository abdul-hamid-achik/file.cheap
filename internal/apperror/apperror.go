package apperror

import "errors"

type Error struct {
	Code      string
	Message   string
	Internal  error
	Retryable bool
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Internal
}

var (
	// File errors
	ErrFileTooLarge = &Error{
		Code:    "file_too_large",
		Message: "File exceeds the maximum allowed size",
	}

	ErrInvalidFileType = &Error{
		Code:    "invalid_file_type",
		Message: "This file type is not supported",
	}

	ErrFileNotFound = &Error{
		Code:    "file_not_found",
		Message: "File not found",
	}

	ErrUnsupportedFormat = &Error{
		Code:    "unsupported_format",
		Message: "Unsupported file format",
	}

	ErrOutputExists = &Error{
		Code:    "output_exists",
		Message: "Output file already exists (use --overwrite)",
	}

	// Processing errors
	ErrProcessingFailed = &Error{
		Code:    "processing_failed",
		Message: "File processing failed",
	}

	ErrProcessorNotFound = &Error{
		Code:    "processor_not_found",
		Message: "The requested processor is not available",
	}

	// Storage errors
	ErrStorageDownloadFailed = &Error{
		Code:    "storage_download_failed",
		Message: "Failed to download file from storage",
	}

	ErrStorageUploadFailed = &Error{
		Code:    "storage_upload_failed",
		Message: "Failed to upload file to storage",
	}

	// System errors
	ErrInternal = &Error{
		Code:    "internal_error",
		Message: "An unexpected error occurred",
	}

	ErrServiceUnavailable = &Error{
		Code:    "service_unavailable",
		Message: "Service temporarily unavailable",
	}

	ErrDependencyMissing = &Error{
		Code:    "dependency_missing",
		Message: "Required external dependency not found",
	}
)

func New(code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

func Wrap(err error, appErr *Error) *Error {
	return &Error{
		Code:     appErr.Code,
		Message:  appErr.Message,
		Internal: err,
	}
}

func WrapWithMessage(err error, code, message string) *Error {
	return &Error{
		Code:     code,
		Message:  message,
		Internal: err,
	}
}

func Is(err error, target *Error) bool {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code == target.Code
	}
	return false
}

func Code(err error) string {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ErrInternal.Code
}

// IsRetryable returns whether the error indicates the operation can be retried.
func IsRetryable(err error) bool {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Retryable
	}
	return true
}

// WithRetryable creates a new error with the retryable flag set.
func WithRetryable(err *Error, retryable bool) *Error {
	return &Error{
		Code:      err.Code,
		Message:   err.Message,
		Internal:  err.Internal,
		Retryable: retryable,
	}
}
