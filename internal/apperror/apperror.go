package apperror

import (
	"errors"
	"net/http"
)

type Error struct {
	Code       string
	Message    string
	StatusCode int
	Internal   error
	Retryable  bool // Whether the operation can be retried
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Internal
}

var (
	ErrNotFound = &Error{
		Code:       "not_found",
		Message:    "The requested resource was not found",
		StatusCode: http.StatusNotFound,
	}

	ErrUnauthorized = &Error{
		Code:       "unauthorized",
		Message:    "Authentication required",
		StatusCode: http.StatusUnauthorized,
	}

	ErrForbidden = &Error{
		Code:       "forbidden",
		Message:    "You don't have permission to access this resource",
		StatusCode: http.StatusForbidden,
	}

	ErrBadRequest = &Error{
		Code:       "bad_request",
		Message:    "Invalid request",
		StatusCode: http.StatusBadRequest,
	}

	ErrInvalidCredentials = &Error{
		Code:       "invalid_credentials",
		Message:    "Invalid email or password",
		StatusCode: http.StatusUnauthorized,
	}

	ErrEmailTaken = &Error{
		Code:       "email_taken",
		Message:    "An account with this email already exists",
		StatusCode: http.StatusConflict,
	}

	ErrInvalidToken = &Error{
		Code:       "invalid_token",
		Message:    "Invalid or expired token",
		StatusCode: http.StatusBadRequest,
	}

	ErrFileTooLarge = &Error{
		Code:       "file_too_large",
		Message:    "The uploaded file exceeds the maximum allowed size",
		StatusCode: http.StatusRequestEntityTooLarge,
	}

	ErrInvalidFileType = &Error{
		Code:       "invalid_file_type",
		Message:    "This file type is not supported",
		StatusCode: http.StatusBadRequest,
	}

	ErrRateLimited = &Error{
		Code:       "rate_limited",
		Message:    "Too many requests. Please try again later",
		StatusCode: http.StatusTooManyRequests,
	}

	ErrInternal = &Error{
		Code:       "internal_error",
		Message:    "An unexpected error occurred. Please try again later",
		StatusCode: http.StatusInternalServerError,
	}

	ErrServiceUnavailable = &Error{
		Code:       "service_unavailable",
		Message:    "Service temporarily unavailable. Please try again later",
		StatusCode: http.StatusServiceUnavailable,
	}

	ErrWeakPassword = &Error{
		Code:       "weak_password",
		Message:    "Password must be at least 8 characters long",
		StatusCode: http.StatusBadRequest,
	}

	ErrPasswordMismatch = &Error{
		Code:       "password_mismatch",
		Message:    "Passwords do not match",
		StatusCode: http.StatusBadRequest,
	}

	ErrAccountExists = &Error{
		Code:       "account_exists",
		Message:    "An account with this email already exists",
		StatusCode: http.StatusConflict,
	}

	ErrOAuthOnly = &Error{
		Code:       "oauth_only",
		Message:    "This account uses social login. Please sign in with your social account",
		StatusCode: http.StatusBadRequest,
	}

	// Processing errors
	ErrProcessingFailed = &Error{
		Code:       "processing_failed",
		Message:    "File processing failed",
		StatusCode: http.StatusInternalServerError,
	}

	ErrProcessorNotFound = &Error{
		Code:       "processor_not_found",
		Message:    "The requested processor is not available",
		StatusCode: http.StatusBadRequest,
	}

	ErrStorageDownloadFailed = &Error{
		Code:       "storage_download_failed",
		Message:    "Failed to download file from storage",
		StatusCode: http.StatusInternalServerError,
	}

	ErrStorageUploadFailed = &Error{
		Code:       "storage_upload_failed",
		Message:    "Failed to upload file to storage",
		StatusCode: http.StatusInternalServerError,
	}

	// Webhook errors
	ErrWebhookDeliveryFailed = &Error{
		Code:       "webhook_delivery_failed",
		Message:    "Webhook delivery failed",
		StatusCode: http.StatusBadGateway,
	}

	ErrWebhookMaxRetries = &Error{
		Code:       "webhook_max_retries",
		Message:    "Webhook delivery failed after maximum retry attempts",
		StatusCode: http.StatusBadGateway,
	}

	ErrWebhookTimeout = &Error{
		Code:       "webhook_timeout",
		Message:    "Webhook delivery timed out",
		StatusCode: http.StatusGatewayTimeout,
	}

	// Job errors
	ErrJobNotFound = &Error{
		Code:       "job_not_found",
		Message:    "Processing job not found",
		StatusCode: http.StatusNotFound,
	}

	ErrInvalidJobPayload = &Error{
		Code:       "invalid_job_payload",
		Message:    "Invalid job payload",
		StatusCode: http.StatusBadRequest,
	}
)

func New(code, message string, statusCode int) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
	}
}

func Wrap(err error, appErr *Error) *Error {
	return &Error{
		Code:       appErr.Code,
		Message:    appErr.Message,
		StatusCode: appErr.StatusCode,
		Internal:   err,
	}
}

func WrapWithMessage(err error, code, message string, statusCode int) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		StatusCode: statusCode,
		Internal:   err,
	}
}

func Is(err error, target *Error) bool {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code == target.Code
	}
	return false
}

func StatusCode(err error) int {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.StatusCode
	}
	return http.StatusInternalServerError
}

func SafeMessage(err error) string {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Message
	}
	return ErrInternal.Message
}

func Code(err error) string {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ErrInternal.Code
}

// IsRetryable returns whether the error indicates the operation can be retried
func IsRetryable(err error) bool {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Retryable
	}
	// By default, unknown errors are considered retryable
	return true
}

// WithRetryable creates a new error with the retryable flag set
func WithRetryable(err *Error, retryable bool) *Error {
	return &Error{
		Code:       err.Code,
		Message:    err.Message,
		StatusCode: err.StatusCode,
		Internal:   err.Internal,
		Retryable:  retryable,
	}
}
