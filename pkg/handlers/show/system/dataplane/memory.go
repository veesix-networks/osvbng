package dataplane

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(NewMemoryHandler)
	state.RegisterMetric(statepaths.SystemDataplaneMemory, paths.SystemDataplaneMemory)
}

type MemoryHandler struct {
	southbound southbound.Southbound
}

func NewMemoryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &MemoryHandler{southbound: deps.Southbound}
}

func (h *MemoryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return h.southbound.GetMemoryStats()
}

func (h *MemoryHandler) PathPattern() paths.Path {
	return paths.SystemDataplaneMemory
}

func (h *MemoryHandler) Dependencies() []paths.Path {
	return nil
}
