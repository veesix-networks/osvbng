package state

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
)

type CachedShowCollector struct {
	cachePath   string
	handlerPath string
	handler     show.Handler
	store       cache.Cache
	config      CollectorConfig
	logger      *slog.Logger
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func NewCachedShowCollector(
	cachePath string,
	handlerPath string,
	registry show.Registry,
	store cache.Cache,
	config CollectorConfig,
	logger *slog.Logger,
) (*CachedShowCollector, error) {
	handler, err := registry.GetHandler(handlerPath)
	if err != nil {
		return nil, err
	}

	return &CachedShowCollector{
		cachePath:   cachePath,
		handlerPath: handlerPath,
		handler:     handler,
		store:       store,
		config:      config,
		logger:      logger,
	}, nil
}

func (c *CachedShowCollector) Name() string {
	return c.cachePath
}

func (c *CachedShowCollector) Paths() []string {
	return []string{c.cachePath}
}

func (c *CachedShowCollector) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.config.Interval)
		defer ticker.Stop()

		if err := c.collectNow(ctx); err != nil {
			c.logger.Error("Initial collection failed", "collector", c.cachePath, "error", err)
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.collectNow(ctx); err != nil {
					c.logger.Error("Collection failed", "collector", c.cachePath, "error", err)
				}
			}
		}
	}()

	return nil
}

func (c *CachedShowCollector) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.wg.Wait()
	return nil
}

func (c *CachedShowCollector) collectNow(ctx context.Context) error {
	req := &show.Request{Path: c.handlerPath}
	result, err := c.handler.Collect(ctx, req)
	if err != nil {
		return err
	}

	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	fullPath := c.config.PathPrefix + c.cachePath
	if err := c.store.Set(ctx, fullPath, data, c.config.TTL); err != nil {
		c.logger.Error("Failed to store metric", "path", fullPath, "error", err)
		return err
	}

	c.logger.Debug("Stored metric", "path", fullPath, "bytes", len(data))
	return nil
}

type PathLike interface {
	String() string
}

func RegisterMetric(cachePath PathLike, handlerPath PathLike) {
	defaultRegistry.mu.Lock()
	defer defaultRegistry.mu.Unlock()

	cachePathStr := cachePath.String()
	handlerPathStr := handlerPath.String()

	defaultRegistry.factories[cachePathStr] = func(deps *CollectorDeps) (MetricCollector, error) {
		return NewCachedShowCollector(
			cachePathStr,
			handlerPathStr,
			deps.ShowRegistry,
			deps.Cache,
			deps.Config,
			deps.Logger,
		)
	}
}
