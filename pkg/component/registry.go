package component

import (
	"fmt"
	"sync"
)

type Factory func(deps Dependencies) (Component, error)

type FactoryWithConfig func(config interface{}, deps Dependencies) (Component, error)

type Metadata struct {
	Name       string
	Author     string
	Version    string
	Namespace  string
	Factory    Factory
	ConfigType interface{}
}

var (
	registry         = make(map[string]Factory)
	metadataRegistry = make(map[string]*Metadata)
	mu               sync.RWMutex
)

func Register(namespace string, factory Factory, opts ...func(*Metadata)) {
	mu.Lock()
	defer mu.Unlock()

	if _, exists := registry[namespace]; exists {
		panic(fmt.Sprintf("component %s already registered", namespace))
	}

	meta := &Metadata{
		Name:      namespace,
		Namespace: namespace,
		Factory:   factory,
	}

	for _, opt := range opts {
		opt(meta)
	}

	registry[namespace] = factory
	metadataRegistry[namespace] = meta
}

func WithAuthor(author string) func(*Metadata) {
	return func(m *Metadata) {
		m.Author = author
	}
}

func WithVersion(version string) func(*Metadata) {
	return func(m *Metadata) {
		m.Version = version
	}
}

func WithConfig(configType interface{}) func(*Metadata) {
	return func(m *Metadata) {
		m.ConfigType = configType
	}
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

func GetMetadata(name string) (*Metadata, bool) {
	mu.RLock()
	defer mu.RUnlock()

	meta, exists := metadataRegistry[name]
	return meta, exists
}

func AllMetadata() []*Metadata {
	mu.RLock()
	defer mu.RUnlock()

	metas := make([]*Metadata, 0, len(metadataRegistry))
	for _, meta := range metadataRegistry {
		metas = append(metas, meta)
	}
	return metas
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
		if comp != nil {
			components = append(components, comp)
		}
	}

	return components, nil
}
