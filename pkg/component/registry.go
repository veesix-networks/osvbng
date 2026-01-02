package component

import (
	"fmt"
	"sync"
)

type Factory func(deps Dependencies) (Component, error)

var (
	registry = make(map[string]Factory)
	mu       sync.RWMutex
)

func Register(name string, factory Factory) {
	mu.Lock()
	defer mu.Unlock()

	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("component %s already registered", name))
	}

	registry[name] = factory
}

func Get(name string) (Factory, bool) {
	mu.RLock()
	defer mu.RUnlock()

	factory, exists := registry[name]
	return factory, exists
}

func List() []string {
	mu.RLock()
	defer mu.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

func LoadAll(deps Dependencies) ([]Component, error) {
	mu.RLock()
	defer mu.RUnlock()

	components := make([]Component, 0, len(registry))
	for name, factory := range registry {
		comp, err := factory(deps)
		if err != nil {
			return nil, fmt.Errorf("failed to create component %s: %w", name, err)
		}
		components = append(components, comp)
	}

	return components, nil
}
