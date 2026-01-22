package bgp

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(NewBGPNeighborsHandler)

	state.RegisterMetric(statepaths.ProtocolsBGPNeighbors, paths.ProtocolsBGPNeighbors)
}

type BGPNeighborsHandler struct {
	routing *routing.Component
}

func NewBGPNeighborsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPNeighborsHandler{
		routing: deps.Routing,
	}
}

func (h *BGPNeighborsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("Routing component not available")
	}

	wildcards, err := paths.Extract(req.Path, paths.ProtocolsBGPNeighbors)
	if err != nil {
		return nil, fmt.Errorf("extract neighbor IP: %w", err)
	}

	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}

	neighborIP := wildcards[0]
	return h.routing.GetBGPNeighbor(neighborIP)
}

func (h *BGPNeighborsHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighbors
}

func (h *BGPNeighborsHandler) Dependencies() []paths.Path {
	return nil
}
