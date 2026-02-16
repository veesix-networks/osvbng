package dataplane

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound/vpp"
)

func init() {
	show.RegisterFactory(NewStatsHandler)
}

type StatsHandler struct {
	southbound *vpp.VPP
}

func NewStatsHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &StatsHandler{southbound: deps.Southbound}
}

func (h *StatsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return h.southbound.GetDataplaneStats()
}

func (h *StatsHandler) PathPattern() paths.Path {
	return paths.SystemDataplaneStats
}

func (h *StatsHandler) Dependencies() []paths.Path {
	return nil
}
