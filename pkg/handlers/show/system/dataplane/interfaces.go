package dataplane

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(NewInterfacesHandler)
	state.RegisterMetric(statepaths.SystemDataplaneInterfaces, paths.SystemDataplaneInterfaces)
}

type InterfacesHandler struct {
	southbound *southbound.VPP
}

func NewInterfacesHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &InterfacesHandler{southbound: deps.Southbound}
}

func (h *InterfacesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	return h.southbound.GetInterfaceStats()
}

func (h *InterfacesHandler) PathPattern() paths.Path {
	return paths.SystemDataplaneInterfaces
}

func (h *InterfacesHandler) Dependencies() []paths.Path {
	return nil
}
