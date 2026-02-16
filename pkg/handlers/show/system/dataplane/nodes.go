package dataplane

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound/vpp"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(NewNodesHandler)
	state.RegisterMetric(statepaths.SystemDataplaneNodes, paths.SystemDataplaneNodes)
}

type NodesHandler struct {
	southbound *vpp.VPP
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
