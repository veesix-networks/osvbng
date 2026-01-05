package bgp

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/show/handlers"
	"github.com/veesix-networks/osvbng/pkg/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	handlers.RegisterFactory(NewBGPStatisticsHandler)
	handlers.RegisterFactory(NewBGPIPv6StatisticsHandler)

	state.RegisterMetric(statepaths.ProtocolsBGPStatistics, paths.ProtocolsBGPStatistics)
	state.RegisterMetric(statepaths.ProtocolsBGPIPv6Statistics, paths.ProtocolsBGPIPv6Statistics)
}

type BGPStatisticsHandler struct {
	routing *routing.Component
}

func NewBGPStatisticsHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &BGPStatisticsHandler{
		routing: deps.Routing,
	}
}

func (h *BGPStatisticsHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("Routing component not available")
	}
	return h.routing.GetBGPStatistics(true)
}

func (h *BGPStatisticsHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPStatistics
}

func (h *BGPStatisticsHandler) Dependencies() []paths.Path {
	return nil
}

type BGPIPv6StatisticsHandler struct {
	routing *routing.Component
}

func NewBGPIPv6StatisticsHandler(deps *handlers.ShowDeps) handlers.ShowHandler {
	return &BGPIPv6StatisticsHandler{
		routing: deps.Routing,
	}
}

func (h *BGPIPv6StatisticsHandler) Collect(ctx context.Context, req *handlers.ShowRequest) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("Routing component not available")
	}
	return h.routing.GetBGPStatistics(false)
}

func (h *BGPIPv6StatisticsHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv6Statistics
}

func (h *BGPIPv6StatisticsHandler) Dependencies() []paths.Path {
	return nil
}
