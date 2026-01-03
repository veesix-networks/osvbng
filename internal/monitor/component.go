package monitor

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/state"
)

type Component struct {
	*component.Base

	logger *slog.Logger

	cache             cache.Cache
	collectorRegistry *state.CollectorRegistry
	collectors        []state.MetricCollector
	collectorConfig   state.CollectorConfig
	enabledCollectors []string
}

type Config struct {
	Cache             cache.Cache
	CollectorRegistry *state.CollectorRegistry
	CollectorConfig   state.CollectorConfig
	EnabledCollectors []string
}

func New(cfg Config) *Component {
	return &Component{
		Base:              component.NewBase("monitor"),
		logger:            logger.Component("monitor"),
		cache:             cfg.Cache,
		collectorRegistry: cfg.CollectorRegistry,
		collectorConfig:   cfg.CollectorConfig,
		enabledCollectors: cfg.EnabledCollectors,
	}
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting monitoring component")

	if c.collectorRegistry != nil && c.cache != nil {
		collectors, err := c.collectorRegistry.CreateCollectors(&state.CollectorDeps{
			Cache:  c.cache,
			Config: c.collectorConfig,
			Logger: c.logger,
		}, c.enabledCollectors)
		if err != nil {
			return fmt.Errorf("failed to create state collectors: %w", err)
		}

		c.collectors = collectors
		for _, collector := range c.collectors {
			if err := collector.Start(ctx); err != nil {
				return fmt.Errorf("failed to start collector %s: %w", collector.Name(), err)
			}
			c.logger.Info("Started state collector", "name", collector.Name())
		}
	}

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping monitoring component")

	for _, collector := range c.collectors {
		if err := collector.Stop(); err != nil {
			c.logger.Warn("Failed to stop collector", "name", collector.Name(), "error", err)
		}
	}

	c.StopContext()

	return nil
}
