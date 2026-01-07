package system

import (
	"context"

	confhandlers "github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

type ConfHandlersHandler struct {
	deps *handlers.ShowDeps
}

func init() {
	handlers.RegisterFactory(func(deps *handlers.ShowDeps) handlers.ShowHandler {
		return &ConfHandlersHandler{deps: deps}
	})
}

func (h *ConfHandlersHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	registry := confhandlers.NewRegistry()
	registry.AutoRegisterAll(&confhandlers.ConfDeps{})

	allPaths := registry.GetAllPaths()
	result := make([]string, len(allPaths))
	for i, p := range allPaths {
		result[i] = p.String()
	}

	return result, nil
}

func (h *ConfHandlersHandler) PathPattern() paths.Path {
	return paths.SystemConfHandlers
}

func (h *ConfHandlersHandler) Dependencies() []paths.Path {
	return nil
}
