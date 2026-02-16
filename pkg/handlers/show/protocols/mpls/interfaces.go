package mpls

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound/vpp"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(NewMPLSInterfacesHandler)
	state.RegisterMetric(statepaths.ProtocolsMPLSInterfaces, paths.ProtocolsMPLSInterfaces)
}

type MPLSInterfacesHandler struct {
	vpp *vpp.VPP
}

func NewMPLSInterfacesHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &MPLSInterfacesHandler{vpp: deps.Southbound}
}

func (h *MPLSInterfacesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.vpp == nil {
		return nil, fmt.Errorf("southbound not available")
	}
	return h.vpp.GetMPLSInterfaces()
}

func (h *MPLSInterfacesHandler) PathPattern() paths.Path {
	return paths.ProtocolsMPLSInterfaces
}

func (h *MPLSInterfacesHandler) Dependencies() []paths.Path {
	return nil
}
