package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

type ShowHandlersHandler struct {
	deps *handlers.ShowDeps
}

func init() {
	handlers.RegisterFactory(func(deps *handlers.ShowDeps) handlers.ShowHandler {
		return &ShowHandlersHandler{deps: deps}
	})
}

func (h *ShowHandlersHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	registry := handlers.NewRegistry()
	registry.AutoRegisterAll(&handlers.ShowDeps{})

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
