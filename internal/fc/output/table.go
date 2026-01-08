package output

import (
	"fmt"
	"io"
	"os"
	"strings"
)

type Table struct {
	out     io.Writer
	headers []string
	rows    [][]string
	quiet   bool
}

func NewTable(headers []string, quiet bool) *Table {
	return NewTableWriter(os.Stdout, headers, quiet)
}

func NewTableWriter(out io.Writer, headers []string, quiet bool) *Table {
	return &Table{
		out:     out,
		headers: headers,
		rows:    make([][]string, 0),
		quiet:   quiet,
	}
}

func (t *Table) Append(row []string) {
	t.rows = append(t.rows, row)
}

func (t *Table) Render() {
	if t.quiet {
		return
	}

	colWidths := make([]int, len(t.headers))
	for i, h := range t.headers {
		colWidths[i] = len(h)
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	printRow := func(cells []string) {
		parts := make([]string, len(cells))
		for i, cell := range cells {
			if i < len(colWidths) {
				parts[i] = fmt.Sprintf("%-*s", colWidths[i], cell)
			} else {
				parts[i] = cell
			}
		}
		fmt.Fprintln(t.out, strings.Join(parts, "  "))
	}

	printRow(t.headers)
	for _, row := range t.rows {
		printRow(row)
	}
}

func (t *Table) SetColMinWidth(col int, width int) {
}
