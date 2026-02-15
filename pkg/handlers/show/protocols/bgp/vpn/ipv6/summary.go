package ipv6

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
	show.RegisterFactory(NewBGPVPNIPv6SummaryHandler)
	state.RegisterMetric(statepaths.ProtocolsBGPVPNIPv6Summary, paths.ProtocolsBGPVPNIPv6Summary)
}

type BGPVPNIPv6SummaryHandler struct {
	routing *routing.Component
}

func NewBGPVPNIPv6SummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv6SummaryHandler{routing: deps.Routing}
}

func (h *BGPVPNIPv6SummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNv6Summary()
}

func (h *BGPVPNIPv6SummaryHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv6Summary
}

func (h *BGPVPNIPv6SummaryHandler) Dependencies() []paths.Path {
	return nil
}
