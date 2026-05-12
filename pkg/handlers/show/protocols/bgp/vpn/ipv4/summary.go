package ipv4

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/bgp"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewBGPVPNIPv4SummaryHandler)
	telemetry.RegisterMetric[bgp.VPNSummary](paths.ProtocolsBGPVPNIPv4Summary)
}

type BGPVPNIPv4SummaryHandler struct {
	routing *routing.Component
}

func NewBGPVPNIPv4SummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv4SummaryHandler{routing: deps.Routing}
}

func (h *BGPVPNIPv4SummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNv4Summary()
}

func (h *BGPVPNIPv4SummaryHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv4Summary
}

func (h *BGPVPNIPv4SummaryHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPVPNIPv4SummaryHandler) Summary() string {
	return "Show BGP VPNv4 summary"
}

func (h *BGPVPNIPv4SummaryHandler) Description() string {
	return "Display a summary of BGP VPNv4 unicast neighbor sessions."
}
