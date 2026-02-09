package isis

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
	show.RegisterFactory(NewISISNeighborsHandler)

	state.RegisterMetric(statepaths.ProtocolsISISNeighbors, paths.ProtocolsISISNeighbors)
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
