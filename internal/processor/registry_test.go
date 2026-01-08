package processor

import (
	"context"
	"io"
	"sync"
	"testing"
)

// mockProcessor is a test implementation of Processor.
type mockProcessor struct {
	name           string
	supportedTypes []string
	processFunc    func(ctx context.Context, opts *Options, input io.Reader) (*Result, error)
}

func newMockProcessor(name string, types ...string) *mockProcessor {
	return &mockProcessor{
		name:           name,
		supportedTypes: types,
	}
}

func (m *mockProcessor) Name() string             { return m.name }
func (m *mockProcessor) SupportedTypes() []string { return m.supportedTypes }
func (m *mockProcessor) Process(ctx context.Context, opts *Options, input io.Reader) (*Result, error) {
	if m.processFunc != nil {
		return m.processFunc(ctx, opts, input)
	}
	return &Result{}, nil
}

// TestRegistry_Register tests processor registration.
func TestRegistry_Register(t *testing.T) {
	tests := []struct {
		name       string
		processors []*mockProcessor
		wantCount  int
	}{
		{
			name: "register single processor",
			processors: []*mockProcessor{
				newMockProcessor("thumbnail", "image/jpeg"),
			},
			wantCount: 1,
		},
		{
			name: "register multiple processors",
			processors: []*mockProcessor{
				newMockProcessor("thumbnail", "image/jpeg"),
				newMockProcessor("resize", "image/jpeg", "image/png"),
			},
			wantCount: 2,
		},
		{
			name: "register overwrites existing",
			processors: []*mockProcessor{
				newMockProcessor("thumbnail", "image/jpeg"),
				newMockProcessor("thumbnail", "image/png"), // Same name, different types
			},
			wantCount: 1,
		},
		{
			name:       "empty registry",
			processors: []*mockProcessor{},
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()

			for _, p := range tt.processors {
				r.Register(p.name, p)
			}

			if got := len(r.List()); got != tt.wantCount {
				t.Errorf("List() returned %d processors, want %d", got, tt.wantCount)
			}
		})
	}
}

// TestRegistry_Get tests processor retrieval by name.
func TestRegistry_Get(t *testing.T) {
	tests := []struct {
		name      string
		register  []*mockProcessor
		lookup    string
		wantFound bool
		wantName  string
	}{
		{
			name: "get existing processor",
			register: []*mockProcessor{
				newMockProcessor("thumbnail", "image/jpeg"),
			},
			lookup:    "thumbnail",
			wantFound: true,
			wantName:  "thumbnail",
		},
		{
			name: "get non-existent processor",
			register: []*mockProcessor{
				newMockProcessor("thumbnail", "image/jpeg"),
			},
			lookup:    "resize",
			wantFound: false,
		},
		{
			name:      "get from empty registry",
			register:  []*mockProcessor{},
			lookup:    "thumbnail",
			wantFound: false,
		},
		{
			name: "get after overwrite",
			register: []*mockProcessor{
				newMockProcessor("thumbnail", "image/jpeg"),
				newMockProcessor("thumbnail", "image/png"),
			},
			lookup:    "thumbnail",
			wantFound: true,
			wantName:  "thumbnail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()

			for _, p := range tt.register {
				r.Register(p.name, p)
			}

			proc, found := r.Get(tt.lookup)

			if found != tt.wantFound {
				t.Errorf("Get(%q) found = %v, want %v", tt.lookup, found, tt.wantFound)
			}

			if tt.wantFound {
				if proc == nil {
					t.Errorf("Get(%q) returned nil processor", tt.lookup)
				} else if proc.Name() != tt.wantName {
					t.Errorf("Get(%q) processor name = %q, want %q", tt.lookup, proc.Name(), tt.wantName)
				}
			}
		})
	}
}

// TestRegistry_GetForContentType tests lookup by content type.
func TestRegistry_GetForContentType(t *testing.T) {
	tests := []struct {
		name        string
		processors  []*mockProcessor
		contentType string
		wantCount   int
		wantNames   []string
	}{
		{
			name: "single match",
			processors: []*mockProcessor{
				newMockProcessor("thumbnail", "image/jpeg"),
				newMockProcessor("resize", "image/png"),
			},
			contentType: "image/jpeg",
			wantCount:   1,
			wantNames:   []string{"thumbnail"},
		},
		{
			name: "multiple matches",
			processors: []*mockProcessor{
				newMockProcessor("thumbnail", "image/jpeg", "image/png"),
				newMockProcessor("resize", "image/jpeg", "image/png"),
			},
			contentType: "image/jpeg",
			wantCount:   2,
			wantNames:   []string{"thumbnail", "resize"},
		},
		{
			name: "no matches",
			processors: []*mockProcessor{
				newMockProcessor("thumbnail", "image/jpeg"),
			},
			contentType: "video/mp4",
			wantCount:   0,
		},
		{
			name:        "empty registry",
			processors:  []*mockProcessor{},
			contentType: "image/jpeg",
			wantCount:   0,
		},
		{
			name: "multiple content types per processor",
			processors: []*mockProcessor{
				newMockProcessor("image-proc", "image/jpeg", "image/png", "image/gif", "image/webp"),
			},
			contentType: "image/webp",
			wantCount:   1,
			wantNames:   []string{"image-proc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()

			for _, p := range tt.processors {
				r.Register(p.name, p)
			}

			procs := r.GetForContentType(tt.contentType)

			if len(procs) != tt.wantCount {
				t.Errorf("GetForContentType(%q) returned %d, want %d", tt.contentType, len(procs), tt.wantCount)
			}

			if len(tt.wantNames) > 0 {
				names := make(map[string]bool)
				for _, p := range procs {
					names[p.Name()] = true
				}
				for _, wantName := range tt.wantNames {
					if !names[wantName] {
						t.Errorf("GetForContentType(%q) missing processor %q", tt.contentType, wantName)
					}
				}
			}
		})
	}
}

// TestRegistry_List tests listing all registered processor names.
func TestRegistry_List(t *testing.T) {
	tests := []struct {
		name       string
		processors []*mockProcessor
		wantNames  []string
	}{
		{
			name: "list all",
			processors: []*mockProcessor{
				newMockProcessor("thumbnail", "image/jpeg"),
				newMockProcessor("resize", "image/png"),
				newMockProcessor("watermark", "image/jpeg"),
			},
			wantNames: []string{"thumbnail", "resize", "watermark"},
		},
		{
			name:       "empty list",
			processors: []*mockProcessor{},
			wantNames:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()

			for _, p := range tt.processors {
				r.Register(p.name, p)
			}

			names := r.List()

			if len(names) != len(tt.wantNames) {
				t.Errorf("List() returned %d names, want %d", len(names), len(tt.wantNames))
			}

			nameMap := make(map[string]bool)
			for _, n := range names {
				nameMap[n] = true
			}

			for _, want := range tt.wantNames {
				if !nameMap[want] {
					t.Errorf("List() missing %q", want)
				}
			}
		})
	}
}

// TestRegistry_MustGet tests MustGet panic behavior.
func TestRegistry_MustGet(t *testing.T) {
	t.Run("panics for unregistered", func(t *testing.T) {
		r := NewRegistry()

		defer func() {
			if recover() == nil {
				t.Error("MustGet() did not panic for unregistered processor")
			}
		}()

		r.MustGet("nonexistent")
	})

	t.Run("returns registered processor", func(t *testing.T) {
		r := NewRegistry()
		r.Register("thumbnail", newMockProcessor("thumbnail", "image/jpeg"))

		// Should not panic
		proc := r.MustGet("thumbnail")

		if proc.Name() != "thumbnail" {
			t.Errorf("MustGet() returned processor with name %q, want %q", proc.Name(), "thumbnail")
		}
	})
}

// TestRegistry_Concurrent tests thread safety of the registry.
func TestRegistry_Concurrent(t *testing.T) {
	r := NewRegistry()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent registrations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := string(rune('a' + n%26))
			r.Register(name, newMockProcessor(name, "image/jpeg"))
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := string(rune('a' + n%26))
			r.Get(name)
			r.GetForContentType("image/jpeg")
			r.List()
		}(i)
	}

	wg.Wait()

	// Should complete without race conditions or panics
	// With 26 letters and overwrites, we should have 26 processors
	if count := len(r.List()); count == 0 {
		t.Error("Expected some processors to be registered")
	}
}

// TestDefaultRegistry tests the package-level default registry.
func TestDefaultRegistry(t *testing.T) {
	// DefaultRegistry should be initialized and usable
	if DefaultRegistry == nil {
		t.Error("DefaultRegistry is nil")
	}

	// We can register to it
	DefaultRegistry.Register("test-default", newMockProcessor("test-default", "image/jpeg"))

	proc, found := DefaultRegistry.Get("test-default")
	if !found {
		t.Error("DefaultRegistry.Get() failed to find registered processor")
	}
	if proc.Name() != "test-default" {
		t.Errorf("DefaultRegistry processor name = %q, want %q", proc.Name(), "test-default")
	}
}
