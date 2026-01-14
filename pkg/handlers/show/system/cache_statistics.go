package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/cache/memory"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewCacheStatisticsHandler)
}

type CacheStatisticsHandler struct {
	cache cache.Cache
}

type CacheStatistics struct {
	Type       string `json:"type"`
	TotalKeys  int    `json:"total_keys"`
	TotalItems int    `json:"total_items"`
}

func NewCacheStatisticsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &CacheStatisticsHandler{
		cache: deps.Cache,
	}
}

func (h *CacheStatisticsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	stats := &CacheStatistics{
		Type: "unknown",
	}

	if _, ok := h.cache.(*memory.Cache); ok {
		stats.Type = "memory"
	} else {
		stats.Type = "redis"
	}

	keys, _, err := h.cache.Scan(ctx, 0, "*", 100000)
	if err != nil {
		return nil, err
	}

	stats.TotalKeys = len(keys)
	stats.TotalItems = len(keys)

	return stats, nil
}

func (h *CacheStatisticsHandler) PathPattern() paths.Path {
	return paths.SystemCacheStatistics
}

func (h *CacheStatisticsHandler) Dependencies() []paths.Path {
	return nil
}
