package ospf

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
	show.RegisterFactory(NewOSPFNeighborsHandler)

	state.RegisterMetric(statepaths.ProtocolsOSPFNeighbors, paths.ProtocolsOSPFNeighbors)
}

type OSPFNeighborsHandler struct {
	routing *routing.Component
}

func NewOSPFNeighborsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPFNeighborsHandler{
		routing: deps.Routing,
	}
}

func (h *OSPFNeighborsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}

	return h.routing.GetOSPFNeighbors()
}

func (h *OSPFNeighborsHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPFNeighbors
}

func (h *OSPFNeighborsHandler) Dependencies() []paths.Path {
	return nil
}
