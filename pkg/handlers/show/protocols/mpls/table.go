package mpls

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

func init() {
	show.RegisterFactory(NewMPLSTableHandler)
	state.RegisterMetric(statepaths.ProtocolsMPLSTable, paths.ProtocolsMPLSTable)
}

type MPLSTableHandler struct {
	vpp *southbound.VPP
}

func NewMPLSTableHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &MPLSTableHandler{vpp: deps.Southbound}
}

func (h *MPLSTableHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.vpp == nil {
		return nil, fmt.Errorf("southbound not available")
	}
	return h.vpp.GetMPLSRoutes()
}

func (h *MPLSTableHandler) PathPattern() paths.Path {
	return paths.ProtocolsMPLSTable
}

func (h *MPLSTableHandler) Dependencies() []paths.Path {
	return nil
}
