package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

type ConfHandlersHandler struct {
	deps *deps.ShowDeps
}

func init() {
	show.RegisterFactory(func(deps *deps.ShowDeps) show.ShowHandler {
		return &ConfHandlersHandler{deps: deps}
	})
}

func (h *ConfHandlersHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	registry := conf.NewRegistry()
	registry.AutoRegisterAll(&deps.ConfDeps{})

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
