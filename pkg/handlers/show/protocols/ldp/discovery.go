package ldp

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
	show.RegisterFactory(NewLDPDiscoveryHandler)
	state.RegisterMetric(statepaths.ProtocolsLDPDiscovery, paths.ProtocolsLDPDiscovery)
}

type LDPDiscoveryHandler struct {
	routing *routing.Component
}

func NewLDPDiscoveryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &LDPDiscoveryHandler{routing: deps.Routing}
}

func (h *LDPDiscoveryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetLDPDiscovery()
}

func (h *LDPDiscoveryHandler) PathPattern() paths.Path {
	return paths.ProtocolsLDPDiscovery
}

func (h *LDPDiscoveryHandler) Dependencies() []paths.Path {
	return nil
}
