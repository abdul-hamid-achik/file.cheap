//go:build !darwin && !linux

package fslock

import (
	"context"
	"sync"
)

var processLocks sync.Map

// Lock is an acquired process-local fallback lock for unsupported platforms.
type Lock struct {
	token chan struct{}
}

// Acquire waits for a process-local lock while honoring cancellation.
func Acquire(ctx context.Context, path string) (*Lock, error) {
	value, _ := processLocks.LoadOrStore(path, makeToken())
	token := value.(chan struct{})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-token:
		return &Lock{token: token}, nil
	}
}

func makeToken() chan struct{} {
	token := make(chan struct{}, 1)
	token <- struct{}{}
	return token
}

// Release returns the process-local token.
func (l *Lock) Release() error {
	if l != nil && l.token != nil {
		l.token <- struct{}{}
		l.token = nil
	}
	return nil
}
