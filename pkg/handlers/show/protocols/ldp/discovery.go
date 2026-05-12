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
	show.RegisterFactory(NewLDPDiscoveryHandler)
	telemetry.RegisterMetric[ldp.Discovery](paths.ProtocolsLDPDiscovery)
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

func (h *LDPDiscoveryHandler) Summary() string {
	return "Show LDP discovery"
}

func (h *LDPDiscoveryHandler) Description() string {
	return "Display LDP discovery hello adjacencies."
}
