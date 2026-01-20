package processor

import (
	"fmt"
	"sync"
)

type Registry struct {
	processors   map[string]Processor
	contentTypes map[string][]Processor
	mu           sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		processors:   make(map[string]Processor),
		contentTypes: make(map[string][]Processor),
	}
}

func (r *Registry) Register(name string, processor Processor) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.processors[name] = processor
	for _, ct := range processor.SupportedTypes() {
		r.contentTypes[ct] = append(r.contentTypes[ct], processor)
	}
}

func (r *Registry) Get(name string) (Processor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.processors[name]
	return p, ok
}

func (r *Registry) GetForContentType(contentType string) []Processor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.contentTypes[contentType]
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.processors))
	for name := range r.processors {
		names = append(names, name)
	}
	return names
}

func (r *Registry) MustGet(name string) Processor {
	processor, exists := r.Get(name)
	if !exists {
		panic(fmt.Sprintf("processor not registered: %s", name))
	}
	return processor
}

// GetOrError returns a processor by name, or an error if not found.
// This is a safer alternative to MustGet that doesn't panic.
func (r *Registry) GetOrError(name string) (Processor, error) {
	processor, exists := r.Get(name)
	if !exists {
		return nil, fmt.Errorf("processor not registered: %s", name)
	}
	return processor, nil
}

var DefaultRegistry = NewRegistry()
