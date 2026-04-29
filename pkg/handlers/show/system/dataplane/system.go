package dataplane

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	show.RegisterFactory(NewSystemHandler)
}

type SystemHandler struct {
	southbound southbound.Southbound
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

func (h *SystemHandler) Summary() string {
	return "Show VPP system info"
}

func (h *SystemHandler) Description() string {
	return "Display VPP version, uptime, and runtime information from the stats segment."
}
