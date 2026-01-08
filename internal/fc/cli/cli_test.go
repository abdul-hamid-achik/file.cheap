package cli

import (
	"bytes"
	"strings"
	"testing"
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
