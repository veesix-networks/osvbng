package state

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/cache"
)

type DataCollectFunc func(ctx context.Context) ([]byte, error)

type GenericCollector struct {
	name       string
	paths      []string
	collectFn  DataCollectFunc
	store      cache.Cache
	config     CollectorConfig
	logger     *slog.Logger
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

func NewGenericCollector(name string, paths []string, collectFn DataCollectFunc, store cache.Cache, config CollectorConfig, logger *slog.Logger) *GenericCollector {
	return &GenericCollector{
		name:      name,
		paths:     paths,
		collectFn: collectFn,
		store:     store,
		config:    config,
		logger:    logger,
	}
}

func (c *GenericCollector) Name() string {
	return c.name
}

func (c *GenericCollector) Paths() []string {
	return c.paths
}

func (c *GenericCollector) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.config.Interval)
		defer ticker.Stop()

		if err := c.collectNow(ctx); err != nil {
			c.logger.Error("Initial collection failed", "collector", c.name, "error", err)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.collectNow(ctx); err != nil {
					c.logger.Error("Collection failed", "collector", c.name, "error", err)
				}
			}
		}
	}()

	return nil
}

func (c *GenericCollector) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	return nil
}

func (c *GenericCollector) collectNow(ctx context.Context) error {
	data, err := c.collectFn(ctx)
	if err != nil {
		return err
	}

	for _, path := range c.paths {
		fullPath := c.config.PathPrefix + path
		if err := c.store.Set(ctx, fullPath, data, c.config.TTL); err != nil {
			c.logger.Error("Failed to store metric", "path", fullPath, "error", err)
			return err
		}
		c.logger.Debug("Stored metric", "path", fullPath, "bytes", len(data))
	}

	return nil
}
