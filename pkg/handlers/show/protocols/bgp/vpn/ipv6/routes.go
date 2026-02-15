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
	show.RegisterFactory(NewBGPVPNIPv6Handler)
	state.RegisterMetric(statepaths.ProtocolsBGPVPNIPv6, paths.ProtocolsBGPVPNIPv6)
}

type BGPVPNIPv6Handler struct {
	routing *routing.Component
}

func NewBGPVPNIPv6Handler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv6Handler{routing: deps.Routing}
}

func (h *BGPVPNIPv6Handler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNv6Routes()
}

func (h *BGPVPNIPv6Handler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv6
}

func (h *BGPVPNIPv6Handler) Dependencies() []paths.Path {
	return nil
}
