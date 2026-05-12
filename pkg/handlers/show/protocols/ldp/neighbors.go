package ldp

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/ldp"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewLDPNeighborsHandler)
	telemetry.RegisterMetric[ldp.Neighbor](paths.ProtocolsLDPNeighbors)
}

type LDPNeighborsHandler struct {
	routing *routing.Component
}

func NewLDPNeighborsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPNeighborsHandler{routing: deps.Routing}
}

func (h *LDPNeighborsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetLDPNeighbors()
}

func (h *LDPNeighborsHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPNeighbors
}

func (h *LDPNeighborsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *LDPNeighborsHandler) Summary() string {
	return "Show LDP neighbors"
}

func (h *LDPNeighborsHandler) Description() string {
	return "Display LDP neighbor sessions with their transport addresses and state."
}
