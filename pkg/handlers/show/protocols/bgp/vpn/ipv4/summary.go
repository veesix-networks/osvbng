package ipv4

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
	show.RegisterFactory(NewBGPVPNIPv4SummaryHandler)
	state.RegisterMetric(statepaths.ProtocolsBGPVPNIPv4Summary, paths.ProtocolsBGPVPNIPv4Summary)
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
