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
	show.RegisterFactory(NewBGPVPNIPv4Handler)
	state.RegisterMetric(statepaths.ProtocolsBGPVPNIPv4, paths.ProtocolsBGPVPNIPv4)
}

type BGPVPNIPv4Handler struct {
	routing *routing.Component
}

func NewBGPVPNIPv4Handler(deps *deps.ShowDeps) show.ShowHandler {
	return &BGPVPNIPv4Handler{routing: deps.Routing}
}

func (h *BGPVPNIPv4Handler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	return h.routing.GetBGPVPNv4Routes()
}

func (h *BGPVPNIPv4Handler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVPNIPv4
}

func (h *BGPVPNIPv4Handler) Dependencies() []paths.Path {
	return nil
}
