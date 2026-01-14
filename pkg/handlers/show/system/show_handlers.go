package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

type ShowHandlersHandler struct {
	deps *deps.ShowDeps
}

func init() {
	show.RegisterFactory(func(deps *deps.ShowDeps) show.ShowHandler {
		return &ShowHandlersHandler{deps: deps}
	})
}

func (h *ShowHandlersHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	registry := show.NewRegistry()
	registry.AutoRegisterAll(&deps.ShowDeps{})

	allPaths := registry.GetAllPaths()
	result := make([]string, len(allPaths))
	for i, p := range allPaths {
		result[i] = p.String()
	}

	return result, nil
}

func (h *ShowHandlersHandler) PathPattern() paths.Path {
	return paths.SystemShowHandlers
}

func (h *ShowHandlersHandler) Dependencies() []paths.Path {
	return nil
}
