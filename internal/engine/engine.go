package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/file.cheap/internal/processor"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor/image"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor/pdf"
	"github.com/abdul-hamid-achik/file.cheap/internal/processor/video"
)

// Engine orchestrates local file processing through the processor registry.
type Engine struct {
	Registry *processor.Registry
	Config   *processor.Config
}

// Request describes a single processing job.
type Request struct {
	InputPath  string             // path to the source file
	OutputPath string             // optional; auto-generated when empty
	Processor  string             // processor name in the registry
	Options    *processor.Options // processor-specific options
}

// Result captures the outcome of a processing job.
type Result struct {
	InputPath  string
	OutputPath string
	InputSize  int64
	OutputSize int64
	Width      int
	Height     int
	Format     string
	Duration   time.Duration
	Metadata   processor.ResultMetadata
}

// ProcessorInfo is a summary returned by ListProcessors.
type ProcessorInfo struct {
	Name           string
	SupportedTypes []string
}

// New creates an Engine with a fresh registry and the given config.
// Call RegisterDefaults to populate the registry with all built-in processors.
func New(cfg *processor.Config) *Engine {
	if cfg == nil {
		cfg = processor.DefaultConfig()
	}
	return &Engine{
		Registry: processor.NewRegistry(),
		Config:   cfg,
	}
}

// RegisterDefaults registers every built-in processor.
// Video processors are silently skipped when ffmpeg/ffprobe are not on PATH.
func (e *Engine) RegisterDefaults() {
	// Image processors (pure Go, always available).
	e.Registry.Register("resize", image.NewResizeProcessor(e.Config))
	e.Registry.Register("thumbnail", image.NewThumbnailProcessor(e.Config))
	e.Registry.Register("webp", image.NewWebPProcessor(e.Config))
	e.Registry.Register("optimize", image.NewOptimizeProcessor(e.Config))
	e.Registry.Register("convert", image.NewConvertProcessor(e.Config))
	e.Registry.Register("watermark", image.NewWatermarkProcessor(e.Config))
	e.Registry.Register("metadata", image.NewMetadataProcessor(e.Config))

	// PDF processor (needs poppler-utils or mutool at runtime).
	e.Registry.Register("pdf_thumbnail", pdf.NewThumbnailProcessor(e.Config))

	// Video processors – require ffmpeg + ffprobe.
	vcfg := video.DefaultVideoConfig()
	vcfg.Config = e.Config

	if vt, err := video.NewThumbnailProcessor(vcfg); err == nil {
		e.Registry.Register("video_thumbnail", vt)
	}
	if vf, err := video.NewFFmpegProcessor(vcfg); err == nil {
		e.Registry.Register("video_transcode", vf)
	}
}

// DetectContentType reads the first 512 bytes of path and returns the MIME type.
func DetectContentType(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return http.DetectContentType(buf[:n]), nil
}

// ListProcessors returns info about every registered processor.
func (e *Engine) ListProcessors() []ProcessorInfo {
	names := e.Registry.List()
	out := make([]ProcessorInfo, len(names))
	for i, name := range names {
		p, _ := e.Registry.Get(name)
		out[i] = ProcessorInfo{
			Name:           name,
			SupportedTypes: p.SupportedTypes(),
		}
	}
	return out
}

// Process executes a single processing request.
func (e *Engine) Process(ctx context.Context, req *Request) (*Result, error) {
	if req == nil {
		return nil, fmt.Errorf("engine: nil request")
	}

	// Look up processor.
	proc, err := e.Registry.GetOrError(req.Processor)
	if err != nil {
		return nil, err
	}

	// Open input file.
	inFile, err := os.Open(req.InputPath)
	if err != nil {
		return nil, fmt.Errorf("engine: open input: %w", err)
	}
	defer inFile.Close()

	inStat, err := inFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("engine: stat input: %w", err)
	}

	opts := req.Options
	if opts == nil {
		opts = &processor.Options{}
	}

	start := time.Now()

	// Run the processor.
	pResult, err := proc.Process(ctx, opts, inFile)
	if err != nil {
		return nil, fmt.Errorf("engine: %s: %w", req.Processor, err)
	}

	duration := time.Since(start)

	res := &Result{
		InputPath: req.InputPath,
		InputSize: inStat.Size(),
		Width:     pResult.Metadata.Width,
		Height:    pResult.Metadata.Height,
		Format:    pResult.Metadata.Format,
		Duration:  duration,
		Metadata:  pResult.Metadata,
	}

	// Metadata processor has no output data to write.
	if pResult.Data == nil {
		return res, nil
	}

	// Determine output path.
	outPath := req.OutputPath
	if outPath == "" {
		outPath = GenerateOutputPath(req.InputPath, req.Processor, opts)
		if outPath == "" {
			// Processor like metadata that returns data but no file designation.
			return res, nil
		}
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return nil, fmt.Errorf("engine: create output: %w", err)
	}
	defer outFile.Close()

	written, err := io.Copy(outFile, pResult.Data)
	if err != nil {
		return nil, fmt.Errorf("engine: write output: %w", err)
	}

	res.OutputPath = outPath
	res.OutputSize = written

	return res, nil
}

// ProcessBatch runs multiple requests concurrently up to the given concurrency limit.
// It returns one Result (or nil) and one error per request, in the same order.
func (e *Engine) ProcessBatch(ctx context.Context, reqs []*Request, concurrency int) ([]*Result, []error) {
	if concurrency < 1 {
		concurrency = 1
	}

	results := make([]*Result, len(reqs))
	errs := make([]error, len(reqs))

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i, req := range reqs {
		wg.Add(1)
		go func(idx int, r *Request) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			res, err := e.Process(ctx, r)
			results[idx] = res
			errs[idx] = err
		}(i, req)
	}

	wg.Wait()
	return results, errs
}

// FFmpegAvailable reports whether ffmpeg is on PATH.
func FFmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}
