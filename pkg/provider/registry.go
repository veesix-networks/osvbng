package provider

import (
	"fmt"
	"sync"
)

type Factory func(cfg map[string]string) (Provider, error)

type Registry struct {
	providers map[string]Factory
	mu        sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Factory),
	}
}

func (r *Registry) Register(name string, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.providers[name]; exists {
		panic(fmt.Sprintf("provider %s already registered", name))
	}

	r.providers[name] = factory
}

func (r *Registry) Get(name string) (Factory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, exists := r.providers[name]
	return factory, exists
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}
