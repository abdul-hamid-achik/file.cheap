package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestPrinter_Printf(t *testing.T) {
	var buf bytes.Buffer
	p := New(WithOutput(&buf))

	p.Printf("Hello %s", "World")
	if !strings.Contains(buf.String(), "Hello World") {
		t.Errorf("Printf output = %q, want to contain 'Hello World'", buf.String())
	}
}

func TestPrinter_Printf_Quiet(t *testing.T) {
	var buf bytes.Buffer
	p := New(WithOutput(&buf), WithQuiet(true))

	p.Printf("Hello %s", "World")
	if buf.Len() != 0 {
		t.Errorf("Printf with quiet should produce no output, got %q", buf.String())
	}
}

func TestPrinter_Printf_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := New(WithOutput(&buf), WithJSON(true))

	p.Printf("Hello %s", "World")
	if buf.Len() != 0 {
		t.Errorf("Printf with JSON mode should produce no output, got %q", buf.String())
	}
}

func TestPrinter_Success(t *testing.T) {
	var buf bytes.Buffer
	p := New(WithOutput(&buf), WithNoColor(true))

	p.Success("Done!")
	output := buf.String()
	if !strings.Contains(output, "Done!") {
		t.Errorf("Success output = %q, want to contain 'Done!'", output)
	}
}

func TestPrinter_Error(t *testing.T) {
	var buf bytes.Buffer
	p := New(WithErrOutput(&buf), WithNoColor(true))

	p.Error("Something failed")
	output := buf.String()
	if !strings.Contains(output, "Something failed") {
		t.Errorf("Error output = %q, want to contain 'Something failed'", output)
	}
}

func TestPrinter_JSON(t *testing.T) {
	var buf bytes.Buffer
	p := New(WithOutput(&buf))

	data := map[string]string{"key": "value"}
	if err := p.JSON(data); err != nil {
		t.Fatalf("JSON() error = %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("JSON output key = %q, want 'value'", result["key"])
	}
}

func TestPrinter_Summary(t *testing.T) {
	t.Skip("Summary uses color package which writes to stdout directly")
}

func TestTable(t *testing.T) {
	var buf bytes.Buffer
	table := NewTableWriter(&buf, []string{"Name", "Status"}, false)
	table.Append([]string{"file1.jpg", "completed"})
	table.Append([]string{"file2.jpg", "processing"})
	table.Render()

	output := buf.String()
	if !strings.Contains(output, "file1.jpg") {
		t.Errorf("Table output should contain 'file1.jpg', got %q", output)
	}
	if !strings.Contains(output, "completed") {
		t.Errorf("Table output should contain 'completed', got %q", output)
	}
}

func TestTable_Quiet(t *testing.T) {
	var buf bytes.Buffer
	table := NewTableWriter(&buf, []string{"Name", "Status"}, true)
	table.Append([]string{"file1.jpg", "completed"})
	table.Render()

	if buf.Len() != 0 {
		t.Errorf("Table with quiet should produce no output, got %q", buf.String())
	}
}

func TestNewProgress(t *testing.T) {
	p := NewProgress(10, "Testing", ProgressWithQuiet(true))
	p.Increment()
	p.Finish()
	if p.Duration() < 0 {
		t.Error("Duration should be positive")
	}
}

func TestNewSpinner(t *testing.T) {
	s := NewSpinner("Testing", true)
	s.Update("Still testing")
	s.Finish()
	if s.Duration() < 0 {
		t.Error("Duration should be positive")
	}
}
