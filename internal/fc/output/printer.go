package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
)

type Printer struct {
	out     io.Writer
	errOut  io.Writer
	json    bool
	quiet   bool
	noColor bool
}

type Option func(*Printer)

func WithJSON(json bool) Option {
	return func(p *Printer) {
		p.json = json
	}
}

func WithQuiet(quiet bool) Option {
	return func(p *Printer) {
		p.quiet = quiet
	}
}

func WithNoColor(noColor bool) Option {
	return func(p *Printer) {
		p.noColor = noColor
	}
}

func WithOutput(out io.Writer) Option {
	return func(p *Printer) {
		p.out = out
	}
}

func WithErrOutput(errOut io.Writer) Option {
	return func(p *Printer) {
		p.errOut = errOut
	}
}

func New(opts ...Option) *Printer {
	p := &Printer{
		out:    os.Stdout,
		errOut: os.Stderr,
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.noColor {
		color.NoColor = true
	}
	return p
}

var (
	successIcon = color.GreenString("✓")
	errorIcon   = color.RedString("✗")
	warnIcon    = color.YellowString("!")
	infoIcon    = color.CyanString("→")
	indentIcon  = color.HiBlackString("└─")
)

func (p *Printer) IsJSON() bool {
	return p.json
}

func (p *Printer) IsQuiet() bool {
	return p.quiet
}

func (p *Printer) Printf(format string, args ...interface{}) {
	if p.quiet || p.json {
		return
	}
	fmt.Fprintf(p.out, format, args...)
}

func (p *Printer) Println(args ...interface{}) {
	if p.quiet || p.json {
		return
	}
	fmt.Fprintln(p.out, args...)
}

func (p *Printer) Success(format string, args ...interface{}) {
	if p.quiet || p.json {
		return
	}
	fmt.Fprintf(p.out, "%s %s\n", successIcon, fmt.Sprintf(format, args...))
}

func (p *Printer) Error(format string, args ...interface{}) {
	if p.json {
		return
	}
	fmt.Fprintf(p.errOut, "%s %s\n", errorIcon, fmt.Sprintf(format, args...))
}

func (p *Printer) Warn(format string, args ...interface{}) {
	if p.quiet || p.json {
		return
	}
	fmt.Fprintf(p.out, "%s %s\n", warnIcon, fmt.Sprintf(format, args...))
}

func (p *Printer) Info(format string, args ...interface{}) {
	if p.quiet || p.json {
		return
	}
	fmt.Fprintf(p.out, "%s %s\n", infoIcon, fmt.Sprintf(format, args...))
}

func (p *Printer) Indent(format string, args ...interface{}) {
	if p.quiet || p.json {
		return
	}
	fmt.Fprintf(p.out, "  %s %s\n", indentIcon, fmt.Sprintf(format, args...))
}

func (p *Printer) JSON(v interface{}) error {
	enc := json.NewEncoder(p.out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (p *Printer) PrintResult(result interface{}) error {
	if p.json {
		return p.JSON(result)
	}
	return nil
}

func (p *Printer) Header(title string) {
	if p.quiet || p.json {
		return
	}
	fmt.Fprintln(p.out)
	fmt.Fprintf(p.out, "%s\n", color.New(color.Bold).Sprint(title))
	fmt.Fprintln(p.out)
}

func (p *Printer) Section(title string) {
	if p.quiet || p.json {
		return
	}
	fmt.Fprintf(p.out, "\n%s\n", color.New(color.Bold, color.FgCyan).Sprint(title))
}

func (p *Printer) KeyValue(key, value string) {
	if p.quiet || p.json {
		return
	}
	fmt.Fprintf(p.out, "  %s: %s\n", color.HiBlackString(key), value)
}

func (p *Printer) Summary(successful, failed int) {
	if p.quiet || p.json {
		return
	}
	fmt.Fprintln(p.out)
	total := successful + failed
	if failed == 0 {
		color.Green("%d/%d completed successfully\n", successful, total)
	} else {
		color.Yellow("%d/%d completed (%d failed)\n", successful, total, failed)
	}
}

func (p *Printer) FileUploaded(filename, url string, variants map[string]string) {
	if p.quiet || p.json {
		return
	}
	fmt.Fprintf(p.out, "%s %s %s %s\n", successIcon, filename, infoIcon, url)
	for variant, vurl := range variants {
		fmt.Fprintf(p.out, "  %s %s: %s\n", indentIcon, variant, vurl)
	}
}

func (p *Printer) FileFailed(filename string, err error) {
	if p.json {
		return
	}
	fmt.Fprintf(p.errOut, "%s %s: %v\n", errorIcon, filename, err)
}
