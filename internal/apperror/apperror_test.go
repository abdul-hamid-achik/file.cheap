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
	wrapped := WrapWithMessage(innerErr, "stash_error", "Stash connection failed")

	if wrapped.Code != "stash_error" {
		t.Errorf("Code = %q, want %q", wrapped.Code, "stash_error")
	}
	if wrapped.Message != "Stash connection failed" {
		t.Errorf("Message = %q, want %q", wrapped.Message, "Stash connection failed")
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
			err:    ErrStashNotFound,
			target: ErrStashNotFound,
			want:   true,
		},
		{
			name:   "wrapped matching error",
			err:    Wrap(errors.New("inner"), ErrStashNotFound),
			target: ErrStashNotFound,
			want:   true,
		},
		{
			name:   "non-matching error",
			err:    ErrInternal,
			target: ErrStashNotFound,
			want:   false,
		},
		{
			name:   "non-apperror",
			err:    errors.New("regular error"),
			target: ErrStashNotFound,
			want:   false,
		},
		{
			name:   "nil error",
			err:    nil,
			target: ErrStashNotFound,
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
		{"stash not found", ErrStashNotFound, "stash_not_found"},
		{"file not found", ErrFileNotFound, "file_not_found"},
		{"internal", ErrInternal, "internal_error"},
		{"archive failed", ErrArchiveFailed, "archive_failed"},
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

func TestPredefinedErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		wantCode string
	}{
		{"ErrStashNotFound", ErrStashNotFound, "stash_not_found"},
		{"ErrStashExists", ErrStashExists, "stash_exists"},
		{"ErrFileNotFound", ErrFileNotFound, "file_not_found"},
		{"ErrInvalidPath", ErrInvalidPath, "invalid_path"},
		{"ErrArchiveFailed", ErrArchiveFailed, "archive_failed"},
		{"ErrRestoreFailed", ErrRestoreFailed, "restore_failed"},
		{"ErrIndexFailed", ErrIndexFailed, "index_failed"},
		{"ErrDependencyMissing", ErrDependencyMissing, "dependency_missing"},
		{"ErrInternal", ErrInternal, "internal_error"},
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