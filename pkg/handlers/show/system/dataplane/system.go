package dataplane

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound/vpp"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(NewSystemHandler)
	state.RegisterMetric(statepaths.SystemDataplaneSystem, paths.SystemDataplaneSystem)
}

type SystemHandler struct {
	southbound *vpp.VPP
}

func NewSystemHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &SystemHandler{southbound: deps.Southbound}
}

func (h *SystemHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return h.southbound.GetSystemStats()
}

func (h *SystemHandler) PathPattern() paths.Path {
	return paths.SystemDataplaneSystem
}

func (h *SystemHandler) Dependencies() []paths.Path {
	return nil
}
