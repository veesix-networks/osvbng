package metrics

import (
	"context"
	"log/slog"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/veesix-networks/osvbng/pkg/cache"
)

type MetricHandler interface {
	Name() string
	Paths() []string
	Describe(ch chan<- *prometheus.Desc)
	Collect(ctx context.Context, c cache.Cache, ch chan<- prometheus.Metric) error
}

type MetricHandlerFactory func(logger *slog.Logger) (MetricHandler, error)

type MetricHandlerRegistry struct {
	mu        sync.RWMutex
	factories map[string]MetricHandlerFactory
}

var defaultRegistry = &MetricHandlerRegistry{
	factories: make(map[string]MetricHandlerFactory),
}

func DefaultRegistry() *MetricHandlerRegistry {
	return defaultRegistry
}

func Register(name string, factory MetricHandlerFactory) {
	defaultRegistry.RegisterFactory(name, factory)
}

func (r *MetricHandlerRegistry) RegisterFactory(name string, factory MetricHandlerFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

func (r *MetricHandlerRegistry) CreateHandlers(logger *slog.Logger) ([]MetricHandler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handlers := make([]MetricHandler, 0, len(r.factories))
	for name, factory := range r.factories {
		handler, err := factory(logger)
		if err != nil {
			logger.Error("Failed to create metric handler", "name", name, "error", err)
			continue
		}
		handlers = append(handlers, handler)
	}
	return handlers, nil
}
