package state

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/cache"
)

type CollectorConfig struct {
	Interval   time.Duration
	TTL        time.Duration
	PathPrefix string
}

type MetricCollector interface {
	Start(ctx context.Context) error
	Stop() error
	Name() string
	Paths() []string
}

type CollectorFactory func(deps *CollectorDeps) (MetricCollector, error)

type CollectorDeps struct {
	Cache  cache.Cache
	Config CollectorConfig
	Logger *slog.Logger
}

type CollectorRegistry struct {
	mu            sync.RWMutex
	registrations map[string]func(interface{}) CollectorFactory
	factories     map[string]CollectorFactory
	collectors    []MetricCollector
}

var defaultRegistry = &CollectorRegistry{
	registrations: make(map[string]func(interface{}) CollectorFactory),
	factories:     make(map[string]CollectorFactory),
	collectors:    []MetricCollector{},
}

func DefaultRegistry() *CollectorRegistry {
	return defaultRegistry
}

func (r *CollectorRegistry) RegisterType(name string, factoryFunc func(interface{}) CollectorFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registrations[name] = factoryFunc
}

func RegisterType(name string, factoryFunc func(interface{}) CollectorFactory) {
	defaultRegistry.RegisterType(name, factoryFunc)
}

func (r *CollectorRegistry) SetProvider(name string, provider interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()

	factoryFunc, ok := r.registrations[name]
	if !ok {
		return
	}

	r.factories[name] = factoryFunc(provider)
}

func (r *CollectorRegistry) CreateCollectors(deps *CollectorDeps, enabledCollectors []string) ([]MetricCollector, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var collectors []MetricCollector

	if len(enabledCollectors) == 0 {
		for name := range r.factories {
			enabledCollectors = append(enabledCollectors, name)
		}
	}

	enabledMap := make(map[string]bool)
	for _, name := range enabledCollectors {
		enabledMap[name] = true
	}

	for name, factory := range r.factories {
		if !enabledMap[name] {
			continue
		}
		collector, err := factory(deps)
		if err != nil {
			deps.Logger.Error("Failed to create collector", "name", name, "error", err)
			continue
		}
		collectors = append(collectors, collector)
	}

	r.collectors = collectors
	return collectors, nil
}

