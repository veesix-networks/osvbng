package state

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
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
	Cache        cache.Cache
	Config       CollectorConfig
	Logger       *slog.Logger
	ShowRegistry show.Registry
}

type CollectorRegistry struct {
	mu         sync.RWMutex
	factories  map[string]CollectorFactory
	collectors []MetricCollector
}

var defaultRegistry = &CollectorRegistry{
	factories:  make(map[string]CollectorFactory),
	collectors: []MetricCollector{},
}

func DefaultRegistry() *CollectorRegistry {
	return defaultRegistry
}

func (r *CollectorRegistry) CreateCollectors(deps *CollectorDeps, disabledCollectors []string) ([]MetricCollector, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var collectors []MetricCollector

	disabledMap := make(map[string]bool)
	for _, name := range disabledCollectors {
		disabledMap[name] = true
	}

	for name, factory := range r.factories {
		if disabledMap[name] {
			deps.Logger.Info("Skipping disabled collector", "name", name)
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

