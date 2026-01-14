package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewCacheKeysHandler)
}

type CacheKeysHandler struct {
	cache cache.Cache
}

type CacheKeys struct {
	Pattern string   `json:"pattern"`
	Keys    []string `json:"keys"`
	Count   int      `json:"count"`
}

func NewCacheKeysHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &CacheKeysHandler{
		cache: deps.Cache,
	}
}

func (h *CacheKeysHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	pattern := req.Options["pattern"]
	if pattern == "" {
		pattern = "*"
	}

	keys, _, err := h.cache.Scan(ctx, 0, pattern, 100000)
	if err != nil {
		return nil, err
	}

	return &CacheKeys{
		Pattern: pattern,
		Keys:    keys,
		Count:   len(keys),
	}, nil
}

func (h *CacheKeysHandler) PathPattern() paths.Path {
	return paths.SystemCacheKeys
}

func (h *CacheKeysHandler) Dependencies() []paths.Path {
	return nil
}
