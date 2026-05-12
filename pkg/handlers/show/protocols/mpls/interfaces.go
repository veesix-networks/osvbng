package mpls

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

func init() {
	show.RegisterFactory(NewMPLSInterfacesHandler)
	telemetry.RegisterMetric[southbound.MPLSInterfaceInfo](paths.ProtocolsMPLSInterfaces)
}

type MPLSInterfacesHandler struct {
	southbound southbound.Southbound
}

func NewMPLSInterfacesHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &MPLSInterfacesHandler{southbound: deps.Southbound}
}

func (h *MPLSInterfacesHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.southbound == nil {
		return nil, fmt.Errorf("southbound not available")
	}
	return h.southbound.GetMPLSInterfaces()
}

func (h *MPLSInterfacesHandler) PathPattern() paths.Path {
	return paths.ProtocolsMPLSInterfaces
}

func (h *MPLSInterfacesHandler) Dependencies() []paths.Path {
	return nil
}

func (h *MPLSInterfacesHandler) Summary() string {
	return "Show MPLS-enabled interfaces"
}

func (h *MPLSInterfacesHandler) Description() string {
	return "Display interfaces with MPLS forwarding enabled."
}
