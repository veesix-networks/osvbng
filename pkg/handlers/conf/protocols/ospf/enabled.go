package ospf

import (
	"context"
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound/vpp"
)

var ospfMulticastGroups = []net.IP{
	net.ParseIP("224.0.0.5"), // AllSPFRouters
	net.ParseIP("224.0.0.6"), // AllDRouters
}

func init() {
	conf.RegisterFactory(NewOSPFEnabledHandler)
}

type OSPFEnabledHandler struct {
	vpp *vpp.VPP
}

func NewOSPFEnabledHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPFEnabledHandler{
		vpp: deps.Southbound,
	}
}

func (h *OSPFEnabledHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if _, ok := hctx.NewValue.(bool); !ok {
		return fmt.Errorf("expected bool, got %T", hctx.NewValue)
	}
	return nil
}

func (h *OSPFEnabledHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	enabled, _ := hctx.NewValue.(bool)
	if !enabled {
		return nil
	}

	for _, group := range ospfMulticastGroups {
		if err := h.vpp.AddMfibLocalReceiveAllInterfaces(group, 0); err != nil {
			return fmt.Errorf("add OSPF mfib local receive for %s: %w", group, err)
		}
	}

	return nil
}

func (h *OSPFEnabledHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFEnabledHandler) PathPattern() paths.Path {
	return paths.OSPFEnabled
}

func (h *OSPFEnabledHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFEnabledHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
