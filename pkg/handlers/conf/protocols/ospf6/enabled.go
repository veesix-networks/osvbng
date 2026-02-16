package ospf6

import (
	"context"
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

var ospf6MulticastGroups = []net.IP{
	net.ParseIP("ff02::5"), // AllSPFRouters
	net.ParseIP("ff02::6"), // AllDRouters
}

func init() {
	conf.RegisterFactory(NewOSPF6EnabledHandler)
}

type OSPF6EnabledHandler struct {
	vpp *southbound.VPP
}

func NewOSPF6EnabledHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPF6EnabledHandler{
		vpp: deps.Southbound,
	}
}

func (h *OSPF6EnabledHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if _, ok := hctx.NewValue.(bool); !ok {
		return fmt.Errorf("expected bool, got %T", hctx.NewValue)
	}
	return nil
}

func (h *OSPF6EnabledHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	enabled, _ := hctx.NewValue.(bool)
	if !enabled {
		return nil
	}

	for _, group := range ospf6MulticastGroups {
		if err := h.vpp.AddMfibLocalReceive(group, 0); err != nil {
			return fmt.Errorf("add OSPFv3 mfib local receive for %s: %w", group, err)
		}
	}

	return nil
}

func (h *OSPF6EnabledHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6EnabledHandler) PathPattern() paths.Path {
	return paths.OSPF6Enabled
}

func (h *OSPF6EnabledHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6EnabledHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
