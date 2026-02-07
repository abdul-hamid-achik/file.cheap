package apperror

import (
	"errors"
	"testing"
)

func TestError_Error(t *testing.T) {
	err := &Error{
		Code:    "test_error",
		Message: "Test error message",
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
	err := New("custom_code", "Custom message")

	if err.Code != "custom_code" {
		t.Errorf("Code = %q, want %q", err.Code, "custom_code")
	}
	if err.Message != "Custom message" {
		t.Errorf("Message = %q, want %q", err.Message, "Custom message")
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
	wrapped := WrapWithMessage(innerErr, "storage_error", "Storage connection failed")

	if wrapped.Code != "storage_error" {
		t.Errorf("Code = %q, want %q", wrapped.Code, "storage_error")
	}
	if wrapped.Message != "Storage connection failed" {
		t.Errorf("Message = %q, want %q", wrapped.Message, "Storage connection failed")
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
			err:    ErrFileNotFound,
			target: ErrFileNotFound,
			want:   true,
		},
		{
			name:   "wrapped matching error",
			err:    Wrap(errors.New("inner"), ErrFileNotFound),
			target: ErrFileNotFound,
			want:   true,
		},
		{
			name:   "non-matching error",
			err:    ErrInternal,
			target: ErrFileNotFound,
			want:   false,
		},
		{
			name:   "non-apperror",
			err:    errors.New("regular error"),
			target: ErrFileNotFound,
			want:   false,
		},
		{
			name:   "nil error",
			err:    nil,
			target: ErrFileNotFound,
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

func TestCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"file not found", ErrFileNotFound, "file_not_found"},
		{"internal", ErrInternal, "internal_error"},
		{"processing failed", ErrProcessingFailed, "processing_failed"},
		{"dependency missing", ErrDependencyMissing, "dependency_missing"},
		{"custom", New("custom_code", "message"), "custom_code"},
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
		{"non-retryable apperror", WithRetryable(ErrInvalidFileType, false), false},
		{"default apperror without flag is not retryable", ErrInvalidFileType, false},
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
			if result.Code != tt.err.Code {
				t.Errorf("Code = %q, want %q", result.Code, tt.err.Code)
			}
			if result.Message != tt.err.Message {
				t.Errorf("Message = %q, want %q", result.Message, tt.err.Message)
			}
		})
	}
}

func TestPredefinedErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		wantCode string
	}{
		{"ErrFileTooLarge", ErrFileTooLarge, "file_too_large"},
		{"ErrInvalidFileType", ErrInvalidFileType, "invalid_file_type"},
		{"ErrFileNotFound", ErrFileNotFound, "file_not_found"},
		{"ErrUnsupportedFormat", ErrUnsupportedFormat, "unsupported_format"},
		{"ErrOutputExists", ErrOutputExists, "output_exists"},
		{"ErrProcessingFailed", ErrProcessingFailed, "processing_failed"},
		{"ErrProcessorNotFound", ErrProcessorNotFound, "processor_not_found"},
		{"ErrStorageDownloadFailed", ErrStorageDownloadFailed, "storage_download_failed"},
		{"ErrStorageUploadFailed", ErrStorageUploadFailed, "storage_upload_failed"},
		{"ErrInternal", ErrInternal, "internal_error"},
		{"ErrServiceUnavailable", ErrServiceUnavailable, "service_unavailable"},
		{"ErrDependencyMissing", ErrDependencyMissing, "dependency_missing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.wantCode {
				t.Errorf("%s.Code = %q, want %q", tt.name, tt.err.Code, tt.wantCode)
			}
			if tt.err.Message == "" {
				t.Errorf("%s.Message should not be empty", tt.name)
			}
		})
	}
}
