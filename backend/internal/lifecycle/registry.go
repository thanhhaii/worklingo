// Package lifecycle tracks a cancellable context per active session so that
// background work (persona replies, schedulers, timers) stops when a session ends.
package lifecycle

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

type Registry struct {
	root context.Context
	mu   sync.Mutex
	m    map[uuid.UUID]entry
}

type entry struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewRegistry(root context.Context) *Registry {
	return &Registry{root: root, m: map[uuid.UUID]entry{}}
}

// Root returns the application-wide context (cancelled on shutdown).
func (r *Registry) Root() context.Context { return r.root }

// Start returns the session context, creating it if needed.
func (r *Registry) Start(id uuid.UUID) context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.m[id]; ok {
		return e.ctx
	}
	ctx, cancel := context.WithCancel(r.root)
	r.m[id] = entry{ctx, cancel}
	return ctx
}

// Get returns the session context if the session is running.
func (r *Registry) Get(id uuid.UUID) (context.Context, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.m[id]
	return e.ctx, ok
}

// Stop cancels the session context.
func (r *Registry) Stop(id uuid.UUID) {
	r.mu.Lock()
	e, ok := r.m[id]
	delete(r.m, id)
	r.mu.Unlock()
	if ok {
		e.cancel()
	}
}
