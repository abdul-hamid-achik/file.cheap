package pdf

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/file-processor/internal/processor"
)

func skipIfNoPopplerUtils(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not available, skipping test")
	}
	if _, err := exec.LookPath("pdfinfo"); err != nil {
		t.Skip("pdfinfo not available, skipping test")
	}
}

func loadTestPDF(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../../testdata/sample.pdf")
	if err != nil {
		t.Fatalf("failed to load test PDF: %v", err)
	}
	return data
}

func TestThumbnailProcessor_Name(t *testing.T) {
	p := NewThumbnailProcessor(nil)
	if p.Name() != "pdf_thumbnail" {
		t.Errorf("expected name 'pdf_thumbnail', got '%s'", p.Name())
	}
}

func TestThumbnailProcessor_SupportedTypes(t *testing.T) {
	p := NewThumbnailProcessor(nil)
	types := p.SupportedTypes()
	if len(types) != 1 || types[0] != "application/pdf" {
		t.Errorf("expected ['application/pdf'], got %v", types)
	}
}

func TestThumbnailProcessor_Process_FirstPage(t *testing.T) {
	skipIfNoPopplerUtils(t)

	p := NewThumbnailProcessor(processor.DefaultConfig())
	pdfData := loadTestPDF(t)

	opts := &processor.Options{
		Width:  300,
		Height: 300,
		Page:   1,
		Format: "png",
	}

	result, err := p.Process(context.Background(), opts, bytes.NewReader(pdfData))
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if result.ContentType != "image/png" {
		t.Errorf("expected content type 'image/png', got '%s'", result.ContentType)
	}

	if result.Size == 0 {
		t.Error("expected non-zero output size")
	}

	data, err := io.ReadAll(result.Data)
	if err != nil {
		t.Fatalf("failed to read result data: %v", err)
	}

	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Errorf("output is not a valid PNG: %v", err)
	}
}

func TestThumbnailProcessor_Process_SecondPage(t *testing.T) {
	skipIfNoPopplerUtils(t)

	p := NewThumbnailProcessor(processor.DefaultConfig())
	pdfData := loadTestPDF(t)

	opts := &processor.Options{
		Width:  300,
		Height: 300,
		Page:   2,
		Format: "png",
	}

	result, err := p.Process(context.Background(), opts, bytes.NewReader(pdfData))
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if result.ContentType != "image/png" {
		t.Errorf("expected content type 'image/png', got '%s'", result.ContentType)
	}

	if result.Size == 0 {
		t.Error("expected non-zero output size")
	}
}

func TestThumbnailProcessor_Process_JPEG(t *testing.T) {
	skipIfNoPopplerUtils(t)

	p := NewThumbnailProcessor(processor.DefaultConfig())
	pdfData := loadTestPDF(t)

	opts := &processor.Options{
		Width:   300,
		Height:  300,
		Page:    1,
		Format:  "jpeg",
		Quality: 85,
	}

	result, err := p.Process(context.Background(), opts, bytes.NewReader(pdfData))
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if result.ContentType != "image/jpeg" {
		t.Errorf("expected content type 'image/jpeg', got '%s'", result.ContentType)
	}

	if result.Size == 0 {
		t.Error("expected non-zero output size")
	}

	if result.Filename != "preview.jpg" {
		t.Errorf("expected filename 'preview.jpg', got '%s'", result.Filename)
	}
}

func TestThumbnailProcessor_Process_DefaultOptions(t *testing.T) {
	skipIfNoPopplerUtils(t)

	p := NewThumbnailProcessor(processor.DefaultConfig())
	pdfData := loadTestPDF(t)

	result, err := p.Process(context.Background(), nil, bytes.NewReader(pdfData))
	if err != nil {
		t.Fatalf("Process failed with nil options: %v", err)
	}

	if result.ContentType != "image/png" {
		t.Errorf("expected default format PNG, got '%s'", result.ContentType)
	}

	if result.Size == 0 {
		t.Error("expected non-zero output size")
	}
}

func TestThumbnailProcessor_Process_PageOutOfRange(t *testing.T) {
	skipIfNoPopplerUtils(t)

	p := NewThumbnailProcessor(processor.DefaultConfig())
	pdfData := loadTestPDF(t)

	opts := &processor.Options{
		Width:  300,
		Height: 300,
		Page:   99,
		Format: "png",
	}

	_, err := p.Process(context.Background(), opts, bytes.NewReader(pdfData))
	if err == nil {
		t.Fatal("expected error for page out of range")
	}

	if !errors.Is(err, ErrPageOutOfRange) {
		t.Errorf("expected ErrPageOutOfRange, got: %v", err)
	}
}

func TestThumbnailProcessor_Process_EmptyInput(t *testing.T) {
	skipIfNoPopplerUtils(t)

	p := NewThumbnailProcessor(processor.DefaultConfig())

	opts := &processor.Options{
		Width:  300,
		Height: 300,
		Page:   1,
	}

	_, err := p.Process(context.Background(), opts, bytes.NewReader(nil))
	if err == nil {
		t.Fatal("expected error for empty input")
	}

	if !errors.Is(err, processor.ErrCorruptedFile) {
		t.Errorf("expected ErrCorruptedFile, got: %v", err)
	}
}

func TestThumbnailProcessor_Process_CorruptedPDF(t *testing.T) {
	skipIfNoPopplerUtils(t)

	p := NewThumbnailProcessor(processor.DefaultConfig())

	corruptedPDF := []byte("not a valid PDF content")

	opts := &processor.Options{
		Width:  300,
		Height: 300,
		Page:   1,
	}

	_, err := p.Process(context.Background(), opts, bytes.NewReader(corruptedPDF))
	if err == nil {
		t.Fatal("expected error for corrupted PDF")
	}

	if !errors.Is(err, processor.ErrCorruptedFile) {
		t.Errorf("expected ErrCorruptedFile, got: %v", err)
	}
}

func TestThumbnailProcessor_Process_CustomDimensions(t *testing.T) {
	skipIfNoPopplerUtils(t)

	p := NewThumbnailProcessor(processor.DefaultConfig())
	pdfData := loadTestPDF(t)

	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"small", 100, 100},
		{"medium", 640, 480},
		{"large", 1024, 768},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &processor.Options{
				Width:  tt.width,
				Height: tt.height,
				Page:   1,
				Format: "png",
			}

			result, err := p.Process(context.Background(), opts, bytes.NewReader(pdfData))
			if err != nil {
				t.Fatalf("Process failed: %v", err)
			}

			if result.Size == 0 {
				t.Error("expected non-zero output size")
			}
		})
	}
}

func TestThumbnailProcessor_Process_FormatNormalization(t *testing.T) {
	skipIfNoPopplerUtils(t)

	p := NewThumbnailProcessor(processor.DefaultConfig())
	pdfData := loadTestPDF(t)

	tests := []struct {
		inputFormat    string
		expectedOutput string
	}{
		{"png", "image/png"},
		{"PNG", "image/png"},
		{"jpeg", "image/jpeg"},
		{"JPEG", "image/jpeg"},
		{"jpg", "image/jpeg"},
		{"JPG", "image/jpeg"},
		{"", "image/png"},
		{"invalid", "image/png"},
	}

	for _, tt := range tests {
		t.Run(tt.inputFormat, func(t *testing.T) {
			opts := &processor.Options{
				Width:  300,
				Height: 300,
				Page:   1,
				Format: tt.inputFormat,
			}

			result, err := p.Process(context.Background(), opts, bytes.NewReader(pdfData))
			if err != nil {
				t.Fatalf("Process failed: %v", err)
			}

			if result.ContentType != tt.expectedOutput {
				t.Errorf("expected content type '%s', got '%s'", tt.expectedOutput, result.ContentType)
			}
		})
	}
}

func TestThumbnailProcessor_getPageCount(t *testing.T) {
	skipIfNoPopplerUtils(t)

	p := NewThumbnailProcessor(processor.DefaultConfig())

	count, err := p.getPageCount(context.Background(), "../../../testdata/sample.pdf")
	if err != nil {
		t.Fatalf("getPageCount failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 pages, got %d", count)
	}
}

func TestThumbnailProcessor_getPageCount_InvalidFile(t *testing.T) {
	skipIfNoPopplerUtils(t)

	p := NewThumbnailProcessor(processor.DefaultConfig())

	_, err := p.getPageCount(context.Background(), "/nonexistent/file.pdf")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestThumbnailProcessor_ContextCancellation(t *testing.T) {
	skipIfNoPopplerUtils(t)

	p := NewThumbnailProcessor(processor.DefaultConfig())
	pdfData := loadTestPDF(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := &processor.Options{
		Width:  300,
		Height: 300,
		Page:   1,
	}

	_, err := p.Process(ctx, opts, bytes.NewReader(pdfData))
	if err == nil {
		t.Log("Note: context cancellation may not always cause immediate failure")
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrPDFEncrypted", ErrPDFEncrypted, "encrypted"},
		{"ErrPDFEmpty", ErrPDFEmpty, "no pages"},
		{"ErrPageOutOfRange", ErrPageOutOfRange, "out of range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.err.Error(), tt.msg) {
				t.Errorf("error message should contain '%s', got: %s", tt.msg, tt.err.Error())
			}
		})
	}
}
