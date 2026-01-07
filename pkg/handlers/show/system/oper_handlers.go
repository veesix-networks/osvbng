package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	showpaths "github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

type OperHandlersHandler struct {
	deps *deps.ShowDeps
}

func init() {
	show.RegisterFactory(func(deps *deps.ShowDeps) show.ShowHandler {
		return &OperHandlersHandler{deps: deps}
	})
}

func (h *OperHandlersHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	registry := oper.NewRegistry()
	registry.AutoRegisterAll(&deps.OperDeps{})

	allPaths := registry.GetAllPaths()
	result := make([]string, len(allPaths))
	for i, p := range allPaths {
		result[i] = p.String()
	}

	return result, nil
}

func (h *OperHandlersHandler) PathPattern() showpaths.Path {
	return showpaths.SystemOperHandlers
}

func (h *OperHandlersHandler) Dependencies() []showpaths.Path {
	return nil
}
