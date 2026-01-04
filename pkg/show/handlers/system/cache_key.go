package system

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

func init() {
	handlers.RegisterFactory(NewCacheKeyHandler)
}

type CacheKeyHandler struct {
	cache cache.Cache
}

type CacheKeyValue struct {
	Key       string      `json:"key"`
	Value     interface{} `json:"value"`
	RawValue  string      `json:"raw_value"`
	Exists    bool        `json:"exists"`
	ValueType string      `json:"value_type"`
	Error     string      `json:"error,omitempty"`
}

func NewCacheKeyHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &CacheKeyHandler{
		cache: deps.Cache,
	}
}

func (h *CacheKeyHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	key := req.Options["key"]
	if key == "" {
		return nil, fmt.Errorf("key parameter is required")
	}

	result := &CacheKeyValue{
		Key:    key,
		Exists: false,
	}

	value, err := h.cache.Get(ctx, key)
	if err != nil {
		result.Error = err.Error()
		return result, nil
	}

	result.Exists = true
	result.RawValue = string(value)

	var jsonValue interface{}
	if err := json.Unmarshal(value, &jsonValue); err == nil {
		result.Value = jsonValue
		result.ValueType = "json"
	} else {
		var intValue int64
		if _, err := fmt.Sscanf(string(value), "%d", &intValue); err == nil {
			result.Value = intValue
			result.ValueType = "integer"
		} else {
			if t, err := time.Parse(time.RFC3339, string(value)); err == nil {
				result.Value = t
				result.ValueType = "time"
			} else {
				result.Value = string(value)
				result.ValueType = "string"
			}
		}
	}

	return result, nil
}

func (h *CacheKeyHandler) PathPattern() paths.Path {
	return paths.SystemCacheKey
}

func (h *CacheKeyHandler) Dependencies() []paths.Path {
	return nil
}
