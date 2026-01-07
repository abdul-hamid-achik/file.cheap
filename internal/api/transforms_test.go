package api

import (
	"testing"
)

func TestParseTransforms(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantWidth   int
		wantHeight  int
		wantQuality int
		wantFormat  string
		wantCrop    string
		wantWM      string
		wantErr     bool
	}{
		{
			name:    "empty string",
			input:   "",
			wantErr: false,
		},
		{
			name:    "underscore (original)",
			input:   "_",
			wantErr: false,
		},
		{
			name:    "original keyword",
			input:   "original",
			wantErr: false,
		},
		{
			name:      "width only",
			input:     "w_800",
			wantWidth: 800,
			wantErr:   false,
		},
		{
			name:       "height only",
			input:      "h_600",
			wantHeight: 600,
			wantErr:    false,
		},
		{
			name:       "width and height",
			input:      "w_800,h_600",
			wantWidth:  800,
			wantHeight: 600,
			wantErr:    false,
		},
		{
			name:        "quality",
			input:       "q_85",
			wantQuality: 85,
			wantErr:     false,
		},
		{
			name:       "format webp",
			input:      "f_webp",
			wantFormat: "webp",
			wantErr:    false,
		},
		{
			name:       "format jpg",
			input:      "f_jpg",
			wantFormat: "jpg",
			wantErr:    false,
		},
		{
			name:     "crop thumb",
			input:    "c_thumb",
			wantCrop: "thumb",
			wantErr:  false,
		},
		{
			name:    "watermark",
			input:   "wm_Copyright",
			wantWM:  "Copyright",
			wantErr: false,
		},
		{
			name:        "full transform",
			input:       "w_800,h_600,q_85,f_webp,c_fill",
			wantWidth:   800,
			wantHeight:  600,
			wantQuality: 85,
			wantFormat:  "webp",
			wantCrop:    "fill",
			wantErr:     false,
		},
		{
			name:    "invalid width",
			input:   "w_abc",
			wantErr: true,
		},
		{
			name:    "width too small",
			input:   "w_0",
			wantErr: true,
		},
		{
			name:    "width too large",
			input:   "w_20000",
			wantErr: true,
		},
		{
			name:    "invalid format",
			input:   "f_bmp",
			wantErr: true,
		},
		{
			name:    "invalid crop mode",
			input:   "c_invalid",
			wantErr: true,
		},
		{
			name:    "unknown key",
			input:   "x_123",
			wantErr: true,
		},
		{
			name:    "malformed transform",
			input:   "w800",
			wantErr: true,
		},
		{
			name:    "quality out of range",
			input:   "q_200",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := ParseTransforms(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if opts.Width != tt.wantWidth {
				t.Errorf("Width = %d, want %d", opts.Width, tt.wantWidth)
			}
			if opts.Height != tt.wantHeight {
				t.Errorf("Height = %d, want %d", opts.Height, tt.wantHeight)
			}
			if opts.Quality != tt.wantQuality {
				t.Errorf("Quality = %d, want %d", opts.Quality, tt.wantQuality)
			}
			if opts.Format != tt.wantFormat {
				t.Errorf("Format = %q, want %q", opts.Format, tt.wantFormat)
			}
			if opts.Crop != tt.wantCrop {
				t.Errorf("Crop = %q, want %q", opts.Crop, tt.wantCrop)
			}
			if opts.Watermark != tt.wantWM {
				t.Errorf("Watermark = %q, want %q", opts.Watermark, tt.wantWM)
			}
		})
	}
}

func TestTransformOptions_CacheKey(t *testing.T) {
	opts1 := &TransformOptions{Width: 800, Height: 600, Quality: 85}
	opts2 := &TransformOptions{Width: 800, Height: 600, Quality: 85}
	opts3 := &TransformOptions{Width: 800, Height: 600, Quality: 90}

	key1 := opts1.CacheKey()
	key2 := opts2.CacheKey()
	key3 := opts3.CacheKey()

	if key1 != key2 {
		t.Errorf("identical options should produce same cache key: %q != %q", key1, key2)
	}

	if key1 == key3 {
		t.Error("different options should produce different cache keys")
	}

	if len(key1) != 16 {
		t.Errorf("cache key should be 16 chars, got %d", len(key1))
	}
}

func TestTransformOptions_RequiresProcessing(t *testing.T) {
	tests := []struct {
		name string
		opts *TransformOptions
		want bool
	}{
		{
			name: "empty options",
			opts: &TransformOptions{},
			want: false,
		},
		{
			name: "width set",
			opts: &TransformOptions{Width: 800},
			want: true,
		},
		{
			name: "height set",
			opts: &TransformOptions{Height: 600},
			want: true,
		},
		{
			name: "quality set",
			opts: &TransformOptions{Quality: 85},
			want: true,
		},
		{
			name: "format set",
			opts: &TransformOptions{Format: "webp"},
			want: true,
		},
		{
			name: "crop set",
			opts: &TransformOptions{Crop: "thumb"},
			want: true,
		},
		{
			name: "watermark set",
			opts: &TransformOptions{Watermark: "test"},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.RequiresProcessing(); got != tt.want {
				t.Errorf("RequiresProcessing() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTransformOptions_ProcessorName(t *testing.T) {
	tests := []struct {
		name string
		opts *TransformOptions
		want string
	}{
		{
			name: "webp format",
			opts: &TransformOptions{Format: "webp"},
			want: "webp",
		},
		{
			name: "watermark",
			opts: &TransformOptions{Watermark: "test"},
			want: "watermark",
		},
		{
			name: "small thumbnail",
			opts: &TransformOptions{Width: 200, Height: 200},
			want: "thumbnail",
		},
		{
			name: "large resize",
			opts: &TransformOptions{Width: 800, Height: 600},
			want: "resize",
		},
		{
			name: "width only",
			opts: &TransformOptions{Width: 800},
			want: "resize",
		},
		{
			name: "empty",
			opts: &TransformOptions{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.ProcessorName(); got != tt.want {
				t.Errorf("ProcessorName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateTransforms(t *testing.T) {
	tests := []struct {
		name    string
		opts    *TransformOptions
		wantErr bool
	}{
		{
			name:    "valid options",
			opts:    &TransformOptions{Width: 800, Height: 600},
			wantErr: false,
		},
		{
			name:    "dimensions too large",
			opts:    &TransformOptions{Width: 10000, Height: 10000},
			wantErr: true,
		},
		{
			name:    "thumb without both dimensions",
			opts:    &TransformOptions{Width: 200, Crop: "thumb"},
			wantErr: true,
		},
		{
			name:    "valid thumb",
			opts:    &TransformOptions{Width: 200, Height: 200, Crop: "thumb"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTransforms(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransforms() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
