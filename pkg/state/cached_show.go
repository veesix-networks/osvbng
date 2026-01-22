package state

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/paths"
)

type CachedShowCollector struct {
	cachePath   string
	handlerPath string
	handler     show.Handler
	configMgr   *configmgr.ConfigManager
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
	configMgr *configmgr.ConfigManager,
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
		configMgr:   configMgr,
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
					c.logger.Debug("Collection failed", "collector", c.cachePath, "error", err)
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
	if strings.Contains(c.handlerPath, "<") && strings.Contains(c.handlerPath, ">") {
		return c.collectWildcard(ctx)
	}

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

func (c *CachedShowCollector) collectWildcard(ctx context.Context) error {
	wildcardValues, err := c.resolveWildcardsFromConfig()
	if err != nil {
		return err
	}

	for _, values := range wildcardValues {
		encodedPath, err := paths.Build(c.handlerPath, values...)
		if err != nil {
			c.logger.Error("Failed to build path", "pattern", c.handlerPath, "values", values, "error", err)
			continue
		}

		req := &show.Request{Path: encodedPath}
		result, err := c.handler.Collect(ctx, req)
		if err != nil {
			c.logger.Error("Failed to collect wildcard metric", "path", encodedPath, "error", err)
			continue
		}

		data, err := json.Marshal(result)
		if err != nil {
			c.logger.Error("Failed to marshal wildcard metric", "path", encodedPath, "error", err)
			continue
		}

		cachePath := c.config.PathPrefix + encodedPath
		if err := c.store.Set(ctx, cachePath, data, c.config.TTL); err != nil {
			c.logger.Error("Failed to store wildcard metric", "path", cachePath, "error", err)
			continue
		}

		c.logger.Debug("Stored wildcard metric", "path", cachePath, "bytes", len(data))
	}

	return nil
}

func (c *CachedShowCollector) resolveWildcardsFromConfig() ([][]string, error) {
	runningConfig, err := c.configMgr.GetRunning()
	if err != nil {
		return nil, err
	}

	return configmgr.ResolveWildcardKeys(runningConfig, c.handlerPath)
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
			deps.ConfigMgr,
			deps.Cache,
			deps.Config,
			deps.Logger,
		)
	}
}
