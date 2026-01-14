package system

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

type SystemThreadsHandler struct {
	daemons *deps.ShowDeps
}

func init() {
	show.RegisterFactory(func(daemons *deps.ShowDeps) show.ShowHandler {
		return &SystemThreadsHandler{daemons: daemons}
	})
}

func (h *SystemThreadsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return h.daemons.Southbound.GetSystemThreads()
}

func (h *SystemThreadsHandler) PathPattern() paths.Path {
	return paths.SystemThreads
}

func (h *SystemThreadsHandler) Dependencies() []paths.Path {
	return nil
}
