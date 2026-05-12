package ospf6

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/ospf6"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewOSPF6NeighborsHandler)
	telemetry.RegisterMetric[ospf6.Neighbor](paths.ProtocolsOSPF6Neighbors)
}

type OSPF6NeighborsHandler struct {
	routing *routing.Component
}

func NewOSPF6NeighborsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6NeighborsHandler{
		routing: deps.Routing,
	}
}

func (h *OSPF6NeighborsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}

	return h.routing.GetOSPF6Neighbors()
}

func (h *OSPF6NeighborsHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6Neighbors
}

func (h *OSPF6NeighborsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6NeighborsHandler) Summary() string {
	return "Show OSPFv3 neighbors"
}

func (h *OSPF6NeighborsHandler) Description() string {
	return "Display OSPFv3 neighbor adjacencies with their state."
}
