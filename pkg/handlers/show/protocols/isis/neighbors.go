package isis

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/isis"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewISISNeighborsHandler)
	telemetry.RegisterMetric[isis.Area](paths.ProtocolsISISNeighbors)
}

type ISISNeighborsHandler struct {
	routing *routing.Component
}

func NewISISNeighborsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &ISISNeighborsHandler{
		routing: deps.Routing,
	}
}

func (h *ISISNeighborsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}

	return h.routing.GetISISNeighbors()
}

func (h *ISISNeighborsHandler) PathPattern() paths.Path {
	return paths.ProtocolsISISNeighbors
}

func (h *ISISNeighborsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ISISNeighborsHandler) Summary() string {
	return "Show IS-IS neighbors"
}

func (h *ISISNeighborsHandler) Description() string {
	return "Display IS-IS adjacencies with their state and interface."
}
