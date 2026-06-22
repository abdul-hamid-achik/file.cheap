package output

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/schollz/progressbar/v3"
)

type Progress struct {
	bar     *progressbar.ProgressBar
	quiet   bool
	out     io.Writer
	started time.Time
}

type ProgressOption func(*Progress)

func ProgressWithQuiet(quiet bool) ProgressOption {
	return func(p *Progress) {
		p.quiet = quiet
	}
}

func ProgressWithOutput(out io.Writer) ProgressOption {
	return func(p *Progress) {
		p.out = out
	}
}

func NewProgress(total int, description string, opts ...ProgressOption) *Progress {
	p := &Progress{
		out:     os.Stderr,
		started: time.Now(),
	}
	for _, opt := range opts {
		opt(p)
	}

	if p.quiet {
		return p
	}

	p.bar = progressbar.NewOptions(total,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWriter(p.out),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(false),
		progressbar.OptionSetWidth(30),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowCount(),
		progressbar.OptionOnCompletion(func() {
			_, _ = fmt.Fprint(p.out, "\n")
		}),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[green]=[reset]",
			SaucerHead:    "[green]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	return p
}

func (p *Progress) Increment() {
	if p.bar != nil {
		_ = p.bar.Add(1)
	}
}

func (p *Progress) Finish() {
	if p.bar != nil {
		_ = p.bar.Finish()
	}
}

func (p *Progress) Duration() time.Duration {
	return time.Since(p.started)
}

type Spinner struct {
	bar     *progressbar.ProgressBar
	quiet   bool
	out     io.Writer
	started time.Time
}

func NewSpinner(description string, quiet bool) *Spinner {
	s := &Spinner{
		out:     os.Stderr,
		quiet:   quiet,
		started: time.Now(),
	}

	if quiet {
		return s
	}

	s.bar = progressbar.NewOptions(-1,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWriter(s.out),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionOnCompletion(func() {
			_, _ = fmt.Fprint(s.out, "\n")
		}),
	)

	return s
}

func (s *Spinner) Update(description string) {
	if s.bar != nil {
		s.bar.Describe(description)
		_ = s.bar.Add(1)
	}
}

func (s *Spinner) Finish() {
	if s.bar != nil {
		_ = s.bar.Finish()
	}
}

func (s *Spinner) Duration() time.Duration {
	return time.Since(s.started)
}

type ByteProgress struct {
	bar     *progressbar.ProgressBar
	quiet   bool
	out     io.Writer
	started time.Time
}

func NewByteProgress(total int64, description string, quiet bool) *ByteProgress {
	p := &ByteProgress{
		out:     os.Stderr,
		quiet:   quiet,
		started: time.Now(),
	}

	if quiet {
		return p
	}

	p.bar = progressbar.NewOptions64(total,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWriter(p.out),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(30),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionOnCompletion(func() {
			_, _ = fmt.Fprint(p.out, "\n")
		}),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[cyan]=[reset]",
			SaucerHead:    "[cyan]>[reset]",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	return p
}

func (p *ByteProgress) Write(b []byte) (int, error) {
	n := len(b)
	if p.bar != nil {
		_ = p.bar.Add(n)
	}
	return n, nil
}

func (p *ByteProgress) Finish() {
	if p.bar != nil {
		_ = p.bar.Finish()
	}
}
