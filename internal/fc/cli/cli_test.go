package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file.cheap/internal/fc/config"
)

func TestRootCommand(t *testing.T) {
	cmd := rootCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "file.cheap") {
		t.Error("Help output should mention file.cheap")
	}
	if !strings.Contains(output, "upload") {
		t.Error("Help output should mention upload command")
	}
}

func TestVersionFlag(t *testing.T) {
	t.Skip("Version flag test requires isolated command instance")
}

func TestCollectFiles(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		recursive bool
		wantErr   bool
	}{
		{
			name:    "nonexistent file",
			args:    []string{"nonexistent-file-xyz.jpg"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := collectFiles(tt.args, tt.recursive)
			if (err != nil) != tt.wantErr {
				t.Errorf("collectFiles() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsImageFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"photo.jpg", true},
		{"photo.jpeg", true},
		{"photo.png", true},
		{"photo.gif", true},
		{"photo.webp", true},
		{"photo.bmp", true},
		{"photo.svg", true},
		{"photo.tiff", true},
		{"photo.txt", false},
		{"photo.pdf", false},
		{"photo", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isImageFile(tt.path); got != tt.want {
				t.Errorf("isImageFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		max  int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"short", 3, "..."},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			if got := truncate(tt.s, tt.max); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatSize(tt.bytes); got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestParseTransformString(t *testing.T) {
	tests := []struct {
		input     string
		wantName  string
		wantValue string
	}{
		{"webp", "webp", ""},
		{"resize:lg", "resize", "lg"},
		{"watermark:Hello World", "watermark", "Hello World"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, value := parseTransformString(tt.input)
			if name != tt.wantName || value != tt.wantValue {
				t.Errorf("parseTransformString(%q) = (%q, %q), want (%q, %q)",
					tt.input, name, value, tt.wantName, tt.wantValue)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal filename",
			input:    "image.jpg",
			expected: "image.jpg",
		},
		{
			name:     "path traversal attempt",
			input:    "../../../etc/passwd",
			expected: "passwd",
		},
		// On non-Windows, backslashes are stripped but not treated as path separators
		// The important thing is that dangerous characters are removed
		{
			name:     "backslash removal",
			input:    "file\\name.jpg",
			expected: "filename.jpg",
		},
		{
			name:     "absolute path linux",
			input:    "/etc/passwd",
			expected: "passwd",
		},
		{
			name:     "nested path",
			input:    "foo/bar/baz.jpg",
			expected: "baz.jpg",
		},
		{
			name:     "double dots",
			input:    "..",
			expected: "",
		},
		{
			name:     "single dot",
			input:    ".",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "filename with null byte",
			input:    "image\x00.jpg",
			expected: "image.jpg",
		},
		{
			name:     "filename with spaces",
			input:    "my image.jpg",
			expected: "my image.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSafePath(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		baseDir   string
		filename  string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "normal filename",
			baseDir:  tmpDir,
			filename: "image.jpg",
			wantErr:  false,
		},
		{
			name:     "path traversal attack",
			baseDir:  tmpDir,
			filename: "../../../etc/passwd",
			wantErr:  false, // sanitized to "passwd"
		},
		{
			name:      "empty filename after sanitization",
			baseDir:   tmpDir,
			filename:  "..",
			wantErr:   true,
			errSubstr: "invalid filename",
		},
		{
			name:      "only dots",
			baseDir:   tmpDir,
			filename:  ".",
			wantErr:   true,
			errSubstr: "invalid filename",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := safePath(tt.baseDir, tt.filename)
			if tt.wantErr {
				if err == nil {
					t.Errorf("safePath(%q, %q) expected error, got nil", tt.baseDir, tt.filename)
				} else if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("safePath(%q, %q) error = %q, want error containing %q", tt.baseDir, tt.filename, err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("safePath(%q, %q) unexpected error: %v", tt.baseDir, tt.filename, err)
				}
				// Verify result is within baseDir
				if result != "" && !strings.HasPrefix(result, tmpDir) {
					t.Errorf("safePath result %q is not within baseDir %q", result, tmpDir)
				}
			}
		})
	}
}

func TestRequireAuth(t *testing.T) {
	// Save original cfg
	originalCfg := cfg
	defer func() { cfg = originalCfg }()

	tests := []struct {
		name      string
		apiKey    string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "authenticated",
			apiKey:  "test-api-key",
			wantErr: false,
		},
		{
			name:      "not authenticated",
			apiKey:    "",
			wantErr:   true,
			errSubstr: "not authenticated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test config using real config type
			cfg = &config.Config{
				APIKey:  tt.apiKey,
				BaseURL: "https://test.example.com",
			}

			err := requireAuth()
			if tt.wantErr {
				if err == nil {
					t.Error("requireAuth() expected error, got nil")
				} else if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("requireAuth() error = %q, want error containing %q", err.Error(), tt.errSubstr)
				}
			} else if err != nil {
				t.Errorf("requireAuth() unexpected error: %v", err)
			}
		})
	}
}

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{"nil error", "", false},
		{"401 error", "401 Unauthorized", true},
		{"403 error", "403 Forbidden", true},
		{"unauthorized lower", "unauthorized request", true},
		{"Unauthorized upper", "Unauthorized access", true},
		{"network error", "connection refused", false},
		{"500 error", "500 Internal Server Error", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.errStr != "" {
				err = &testError{msg: tt.errStr}
			}
			result := isAuthError(err)
			if result != tt.expected {
				t.Errorf("isAuthError(%q) = %v, want %v", tt.errStr, result, tt.expected)
			}
		})
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
