package auth

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config"
)

type Factory func(*config.Config) (AuthProvider, error)

var registry = make(map[string]Factory)

func Register(name string, factory Factory) {
	registry[name] = factory
}

func Get(name string) (Factory, bool) {
	factory, exists := registry[name]
	return factory, exists
}

func New(name string, cfg *config.Config) (AuthProvider, error) {
	factory, exists := registry[name]
	if !exists {
		return nil, fmt.Errorf("auth provider %s not registered", name)
	}
	return factory(cfg)
}

func List() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}
