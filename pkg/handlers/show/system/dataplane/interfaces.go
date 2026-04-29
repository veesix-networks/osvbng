package dataplane

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	show.RegisterFactory(NewInterfacesHandler)
}

type InterfacesHandler struct {
	southbound southbound.Southbound
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

func (h *InterfacesHandler) Summary() string {
	return "Show VPP interface counters"
}

func (h *InterfacesHandler) Description() string {
	return "Display per-interface packet and byte counters from the VPP stats segment."
}

func (h *InterfacesHandler) SortKey() string {
	return "name"
}
