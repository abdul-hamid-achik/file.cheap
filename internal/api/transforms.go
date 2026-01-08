package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/abdul-hamid-achik/file-processor/internal/processor"
)

type TransformOptions struct {
	Width     int
	Height    int
	Quality   int
	Format    string
	Crop      string
	Watermark string
	Page      int
}

func (t *TransformOptions) ToProcessorOptions() *processor.Options {
	return &processor.Options{
		Width:       t.Width,
		Height:      t.Height,
		Quality:     t.Quality,
		Format:      t.Format,
		Fit:         t.Crop,
		VariantType: t.Watermark,
		Page:        t.Page,
	}
}

func (t *TransformOptions) CacheKey() string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "w=%d,h=%d,q=%d,f=%s,c=%s,wm=%s,p=%d",
		t.Width, t.Height, t.Quality, t.Format, t.Crop, t.Watermark, t.Page)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func (t *TransformOptions) RequiresProcessing() bool {
	return t.Width > 0 || t.Height > 0 || t.Quality > 0 ||
		t.Format != "" || t.Crop != "" || t.Watermark != "" || t.Page > 0
}

func (t *TransformOptions) ProcessorName() string {
	if t.Format == "webp" {
		return "webp"
	}
	if t.Watermark != "" {
		return "watermark"
	}
	if t.Width > 0 && t.Height > 0 && t.Width <= 200 && t.Height <= 200 {
		return "thumbnail"
	}
	if t.Width > 0 || t.Height > 0 {
		return "resize"
	}
	return ""
}

func (t *TransformOptions) ProcessorNameForContentType(contentType string) string {
	if contentType == "application/pdf" {
		return "pdf_thumbnail"
	}
	return t.ProcessorName()
}

func ParseTransforms(s string) (*TransformOptions, error) {
	if s == "" || s == "_" || s == "original" {
		return &TransformOptions{}, nil
	}

	opts := &TransformOptions{}
	parts := strings.Split(s, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		kv := strings.SplitN(part, "_", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid transform format: %s (expected key_value)", part)
		}

		key, value := kv[0], kv[1]

		switch key {
		case "w":
			w, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid width value: %s", value)
			}
			if w < 1 || w > 10000 {
				return nil, fmt.Errorf("width must be between 1 and 10000, got %d", w)
			}
			opts.Width = w

		case "h":
			h, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid height value: %s", value)
			}
			if h < 1 || h > 10000 {
				return nil, fmt.Errorf("height must be between 1 and 10000, got %d", h)
			}
			opts.Height = h

		case "q":
			q, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid quality value: %s", value)
			}
			if q < 1 || q > 100 {
				return nil, fmt.Errorf("quality must be between 1 and 100, got %d", q)
			}
			opts.Quality = q

		case "f":
			value = strings.ToLower(value)
			switch value {
			case "webp", "jpg", "jpeg", "png", "gif":
				opts.Format = value
			default:
				return nil, fmt.Errorf("unsupported format: %s (supported: webp, jpg, png, gif)", value)
			}

		case "c":
			value = strings.ToLower(value)
			switch value {
			case "thumb", "fit", "fill", "cover", "contain":
				opts.Crop = value
			default:
				return nil, fmt.Errorf("unsupported crop mode: %s (supported: thumb, fit, fill, cover, contain)", value)
			}

		case "wm":
			if len(value) > 100 {
				return nil, fmt.Errorf("watermark text too long (max 100 characters)")
			}
			opts.Watermark = value

		case "p":
			p, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid page value: %s", value)
			}
			if p < 1 || p > 9999 {
				return nil, fmt.Errorf("page must be between 1 and 9999, got %d", p)
			}
			opts.Page = p

		default:
			return nil, fmt.Errorf("unknown transform key: %s", key)
		}
	}

	return opts, nil
}

func ValidateTransforms(opts *TransformOptions) error {
	if opts.Width > 0 && opts.Height > 0 {
		if opts.Width*opts.Height > 25000000 {
			return fmt.Errorf("output dimensions too large (max 25 megapixels)")
		}
	}

	if opts.Crop == "thumb" && (opts.Width == 0 || opts.Height == 0) {
		return fmt.Errorf("crop mode 'thumb' requires both width and height")
	}

	return nil
}
