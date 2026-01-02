package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
)

type SystemThreadsHandler struct {
	daemons *handlers.ShowDeps
}

func init() {
	handlers.RegisterFactory(func(daemons *handlers.ShowDeps) handlers.ShowHandler {
		return &SystemThreadsHandler{daemons: daemons}
	})
}

func (h *SystemThreadsHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	return h.daemons.Southbound.GetSystemThreads()
}

func (h *SystemThreadsHandler) PathPattern() paths.Path {
	return paths.SystemThreads
}

func (h *SystemThreadsHandler) Dependencies() []paths.Path {
	return nil
}
