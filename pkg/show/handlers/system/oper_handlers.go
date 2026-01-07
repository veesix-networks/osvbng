package system

import (
	"context"

	operhandlers "github.com/veesix-networks/osvbng/pkg/oper/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	showpaths "github.com/veesix-networks/osvbng/pkg/show/paths"
)

type OperHandlersHandler struct {
	deps *handlers.ShowDeps
}

func init() {
	handlers.RegisterFactory(func(deps *handlers.ShowDeps) handlers.ShowHandler {
		return &OperHandlersHandler{deps: deps}
	})
}

func (h *OperHandlersHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	registry := operhandlers.NewRegistry()
	registry.AutoRegisterAll(&operhandlers.OperDeps{})

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
