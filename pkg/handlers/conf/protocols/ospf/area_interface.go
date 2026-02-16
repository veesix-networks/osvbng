package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewOSPFAreaInterfaceHandler)
}

type OSPFAreaInterfaceHandler struct {
	vpp *southbound.VPP
}

func NewOSPFAreaInterfaceHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPFAreaInterfaceHandler{
		vpp: deps.Southbound,
	}
}

func (h *OSPFAreaInterfaceHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*protocols.OSPFInterfaceConfig)
	if !ok {
		return fmt.Errorf("expected *protocols.OSPFInterfaceConfig, got %T", hctx.NewValue)
	}

	if cfg == nil {
		return nil
	}

	if cfg.Network != "" && !cfg.Network.Valid() {
		return fmt.Errorf("invalid OSPF network type %q", cfg.Network)
	}

	if cfg.HelloInterval != 0 && (cfg.HelloInterval < 1 || cfg.HelloInterval > 65535) {
		return fmt.Errorf("hello-interval %d out of range (1-65535)", cfg.HelloInterval)
	}

	if cfg.DeadInterval != 0 && (cfg.DeadInterval < 1 || cfg.DeadInterval > 65535) {
		return fmt.Errorf("dead-interval %d out of range (1-65535)", cfg.DeadInterval)
	}

	return nil
}

func (h *OSPFAreaInterfaceHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	values, err := paths.OSPFAreaInterface.ExtractWildcards(hctx.Path, 2)
	if err != nil {
		return fmt.Errorf("extract wildcards: %w", err)
	}
	ifName := values[1]

	swIfIndex, ok := h.vpp.GetIfMgr().GetSwIfIndex(ifName)
	if !ok {
		return fmt.Errorf("interface %q not found in ifmgr", ifName)
	}

	for _, group := range ospfMulticastGroups {
		if err := h.vpp.AddMfibInterfaceAccept(group, 0, swIfIndex); err != nil {
			return fmt.Errorf("add OSPF mfib accept for %s on %s: %w", group, ifName, err)
		}
	}

	return nil
}

func (h *OSPFAreaInterfaceHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFAreaInterfaceHandler) PathPattern() paths.Path {
	return paths.OSPFAreaInterface
}

func (h *OSPFAreaInterfaceHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPFEnabled}
}

func (h *OSPFAreaInterfaceHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
