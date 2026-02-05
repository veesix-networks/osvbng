package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type LoggingHandler struct {
	deps *deps.ShowDeps
}

type LoggingInfo struct {
	DefaultLevel string                 `json:"default_level"`
	Levels       map[string]string      `json:"levels"`
}

func init() {
	show.RegisterFactory(func(deps *deps.ShowDeps) show.ShowHandler {
		return &LoggingHandler{deps: deps}
	})
}

func (h *LoggingHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	levels := logger.GetComponentLevels()

	strLevels := make(map[string]string, len(levels))
	for name, level := range levels {
		strLevels[name] = string(level)
	}

	return &LoggingInfo{
		DefaultLevel: string(logger.GetDefaultLevel()),
		Levels:       strLevels,
	}, nil
}

func (h *LoggingHandler) PathPattern() paths.Path {
	return paths.SystemLogging
}

func (h *LoggingHandler) Dependencies() []paths.Path {
	return nil
}
