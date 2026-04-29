package dataplane

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	show.RegisterFactory(NewNodesHandler)
}

type NodesHandler struct {
	southbound southbound.Southbound
}

func NewNodesHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &NodesHandler{southbound: deps.Southbound}
}

func (h *NodesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return h.southbound.GetNodeStats()
}

func (h *NodesHandler) PathPattern() paths.Path {
	return paths.SystemDataplaneNodes
}

func (h *NodesHandler) Dependencies() []paths.Path {
	return nil
}

func (h *NodesHandler) Summary() string {
	return "Show VPP node counters"
}

func (h *NodesHandler) Description() string {
	return "Display per-node packet counters from the VPP stats segment."
}

func (h *NodesHandler) SortKey() string {
	return "name"
}
