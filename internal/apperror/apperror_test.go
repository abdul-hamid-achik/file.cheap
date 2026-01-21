package apperror

import (
	"errors"
	"net/http"
	"testing"
)

func TestError_Error(t *testing.T) {
	err := &Error{
		Code:       "test_error",
		Message:    "Test error message",
		StatusCode: http.StatusBadRequest,
	}

	if got := err.Error(); got != "Test error message" {
		t.Errorf("Error() = %q, want %q", got, "Test error message")
	}
}

func TestError_Unwrap(t *testing.T) {
	innerErr := errors.New("inner error")
	err := &Error{
		Code:     "wrapped_error",
		Message:  "Wrapped error",
		Internal: innerErr,
	}

	if got := err.Unwrap(); got != innerErr {
		t.Errorf("Unwrap() = %v, want %v", got, innerErr)
	}
}

func TestNew(t *testing.T) {
	err := New("custom_code", "Custom message", http.StatusTeapot)

	if err.Code != "custom_code" {
		t.Errorf("Code = %q, want %q", err.Code, "custom_code")
	}
	if err.Message != "Custom message" {
		t.Errorf("Message = %q, want %q", err.Message, "Custom message")
	}
	if err.StatusCode != http.StatusTeapot {
		t.Errorf("StatusCode = %d, want %d", err.StatusCode, http.StatusTeapot)
	}
}

func TestWrap(t *testing.T) {
	innerErr := errors.New("database error")
	wrapped := Wrap(innerErr, ErrInternal)

	if wrapped.Code != ErrInternal.Code {
		t.Errorf("Code = %q, want %q", wrapped.Code, ErrInternal.Code)
	}
	if wrapped.Internal != innerErr {
		t.Errorf("Internal = %v, want %v", wrapped.Internal, innerErr)
	}
	if !errors.Is(wrapped, innerErr) {
		t.Error("errors.Is should return true for wrapped inner error")
	}
}

func TestWrapWithMessage(t *testing.T) {
	innerErr := errors.New("connection refused")
	wrapped := WrapWithMessage(innerErr, "db_error", "Database connection failed", http.StatusServiceUnavailable)

	if wrapped.Code != "db_error" {
		t.Errorf("Code = %q, want %q", wrapped.Code, "db_error")
	}
	if wrapped.Message != "Database connection failed" {
		t.Errorf("Message = %q, want %q", wrapped.Message, "Database connection failed")
	}
	if wrapped.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want %d", wrapped.StatusCode, http.StatusServiceUnavailable)
	}
	if wrapped.Internal != innerErr {
		t.Errorf("Internal = %v, want %v", wrapped.Internal, innerErr)
	}
}

func TestIs(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		target *Error
		want   bool
	}{
		{
			name:   "matching error",
			err:    ErrNotFound,
			target: ErrNotFound,
			want:   true,
		},
		{
			name:   "wrapped matching error",
			err:    Wrap(errors.New("inner"), ErrNotFound),
			target: ErrNotFound,
			want:   true,
		},
		{
			name:   "non-matching error",
			err:    ErrUnauthorized,
			target: ErrNotFound,
			want:   false,
		},
		{
			name:   "non-apperror",
			err:    errors.New("regular error"),
			target: ErrNotFound,
			want:   false,
		},
		{
			name:   "nil error",
			err:    nil,
			target: ErrNotFound,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Is(tt.err, tt.target); got != tt.want {
				t.Errorf("Is() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatusCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"not found", ErrNotFound, http.StatusNotFound},
		{"unauthorized", ErrUnauthorized, http.StatusUnauthorized},
		{"forbidden", ErrForbidden, http.StatusForbidden},
		{"bad request", ErrBadRequest, http.StatusBadRequest},
		{"internal", ErrInternal, http.StatusInternalServerError},
		{"rate limited", ErrRateLimited, http.StatusTooManyRequests},
		{"service unavailable", ErrServiceUnavailable, http.StatusServiceUnavailable},
		{"non-apperror defaults to 500", errors.New("regular error"), http.StatusInternalServerError},
		{"wrapped error preserves code", Wrap(errors.New("inner"), ErrNotFound), http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StatusCode(tt.err); got != tt.want {
				t.Errorf("StatusCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSafeMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"not found", ErrNotFound, ErrNotFound.Message},
		{"unauthorized", ErrUnauthorized, ErrUnauthorized.Message},
		{"custom error", New("test", "Custom message", 400), "Custom message"},
		{"non-apperror returns internal message", errors.New("db error"), ErrInternal.Message},
		{"nil error returns internal message", nil, ErrInternal.Message},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SafeMessage(tt.err); got != tt.want {
				t.Errorf("SafeMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"not found", ErrNotFound, "not_found"},
		{"unauthorized", ErrUnauthorized, "unauthorized"},
		{"forbidden", ErrForbidden, "forbidden"},
		{"bad request", ErrBadRequest, "bad_request"},
		{"internal", ErrInternal, "internal_error"},
		{"custom", New("custom_code", "message", 400), "custom_code"},
		{"non-apperror", errors.New("regular"), "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Code(tt.err); got != tt.want {
				t.Errorf("Code() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error is retryable", nil, true},
		{"regular error is retryable", errors.New("timeout"), true},
		{"retryable apperror", WithRetryable(ErrInternal, true), true},
		{"non-retryable apperror", WithRetryable(ErrBadRequest, false), false},
		{"default apperror without flag is not retryable", ErrBadRequest, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWithRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       *Error
		retryable bool
	}{
		{"set retryable true", ErrInternal, true},
		{"set retryable false", ErrInternal, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := WithRetryable(tt.err, tt.retryable)
			if result.Retryable != tt.retryable {
				t.Errorf("Retryable = %v, want %v", result.Retryable, tt.retryable)
			}
			// Ensure other fields are preserved
			if result.Code != tt.err.Code {
				t.Errorf("Code = %q, want %q", result.Code, tt.err.Code)
			}
			if result.Message != tt.err.Message {
				t.Errorf("Message = %q, want %q", result.Message, tt.err.Message)
			}
			if result.StatusCode != tt.err.StatusCode {
				t.Errorf("StatusCode = %d, want %d", result.StatusCode, tt.err.StatusCode)
			}
		})
	}
}

func TestPredefinedErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        *Error
		wantCode   string
		wantStatus int
	}{
		{"ErrNotFound", ErrNotFound, "not_found", http.StatusNotFound},
		{"ErrUnauthorized", ErrUnauthorized, "unauthorized", http.StatusUnauthorized},
		{"ErrForbidden", ErrForbidden, "forbidden", http.StatusForbidden},
		{"ErrBadRequest", ErrBadRequest, "bad_request", http.StatusBadRequest},
		{"ErrInvalidCredentials", ErrInvalidCredentials, "invalid_credentials", http.StatusUnauthorized},
		{"ErrEmailTaken", ErrEmailTaken, "email_taken", http.StatusConflict},
		{"ErrInvalidToken", ErrInvalidToken, "invalid_token", http.StatusBadRequest},
		{"ErrFileTooLarge", ErrFileTooLarge, "file_too_large", http.StatusRequestEntityTooLarge},
		{"ErrInvalidFileType", ErrInvalidFileType, "invalid_file_type", http.StatusBadRequest},
		{"ErrRateLimited", ErrRateLimited, "rate_limited", http.StatusTooManyRequests},
		{"ErrInternal", ErrInternal, "internal_error", http.StatusInternalServerError},
		{"ErrServiceUnavailable", ErrServiceUnavailable, "service_unavailable", http.StatusServiceUnavailable},
		{"ErrWeakPassword", ErrWeakPassword, "weak_password", http.StatusBadRequest},
		{"ErrPasswordMismatch", ErrPasswordMismatch, "password_mismatch", http.StatusBadRequest},
		{"ErrAccountExists", ErrAccountExists, "account_exists", http.StatusConflict},
		{"ErrOAuthOnly", ErrOAuthOnly, "oauth_only", http.StatusBadRequest},
		{"ErrProcessingFailed", ErrProcessingFailed, "processing_failed", http.StatusInternalServerError},
		{"ErrProcessorNotFound", ErrProcessorNotFound, "processor_not_found", http.StatusBadRequest},
		{"ErrStorageDownloadFailed", ErrStorageDownloadFailed, "storage_download_failed", http.StatusInternalServerError},
		{"ErrStorageUploadFailed", ErrStorageUploadFailed, "storage_upload_failed", http.StatusInternalServerError},
		{"ErrWebhookDeliveryFailed", ErrWebhookDeliveryFailed, "webhook_delivery_failed", http.StatusBadGateway},
		{"ErrWebhookMaxRetries", ErrWebhookMaxRetries, "webhook_max_retries", http.StatusBadGateway},
		{"ErrWebhookTimeout", ErrWebhookTimeout, "webhook_timeout", http.StatusGatewayTimeout},
		{"ErrJobNotFound", ErrJobNotFound, "job_not_found", http.StatusNotFound},
		{"ErrInvalidJobPayload", ErrInvalidJobPayload, "invalid_job_payload", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.wantCode {
				t.Errorf("%s.Code = %q, want %q", tt.name, tt.err.Code, tt.wantCode)
			}
			if tt.err.StatusCode != tt.wantStatus {
				t.Errorf("%s.StatusCode = %d, want %d", tt.name, tt.err.StatusCode, tt.wantStatus)
			}
			if tt.err.Message == "" {
				t.Errorf("%s.Message should not be empty", tt.name)
			}
		})
	}
}
