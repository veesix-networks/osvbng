package iface

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/evpnmgr"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewInterfaceHandler)
}

type InterfaceHandler struct {
	southbound     southbound.Southbound
	dataplaneState operations.DataplaneStateReader
	evpnMirror     *evpnmgr.Manager
}

func NewInterfaceHandler(daemons *deps.ConfDeps) conf.Handler {
	return &InterfaceHandler{
		southbound:     daemons.Southbound,
		dataplaneState: daemons.DataplaneState,
		evpnMirror:     daemons.EVPNMirror,
	}
}

func isEVPNTunnel(hctx *conf.HandlerContext, name string) bool {
	if hctx.Config == nil {
		return false
	}
	iface, ok := hctx.Config.Interfaces[name]
	return ok && iface != nil && iface.Vxlan != nil && iface.Vxlan.EVPNSignaled()
}

func pseudowireOnTransport(hctx *conf.HandlerContext, transport string) string {
	if hctx.Config == nil {
		return ""
	}
	for name, iface := range hctx.Config.Interfaces {
		if iface != nil && iface.Pseudowire != nil && iface.Pseudowire.Transport == transport {
			return name
		}
	}
	return ""
}

func (h *InterfaceHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(*interfaces.InterfaceConfig)
	if !ok {
		return fmt.Errorf("expected *interfaces.InterfaceConfig, got %T", hctx.NewValue)
	}
	return nil
}

func (h *InterfaceHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg := hctx.NewValue.(*interfaces.InterfaceConfig)

	// EVPN-signaled tunnels get a kernel mirror device (FRR derives
	// the VNI advertisement from it) and then fall through to normal
	// creation: the VPP tunnel is created against a placeholder dst so
	// pseudowire binds and subinterfaces wire up in static-tunnel
	// order, and discovery later replaces the dst with the learned
	// remote VTEP.
	if cfg.Vxlan != nil && cfg.Vxlan.EVPNSignaled() && h.evpnMirror != nil {
		if err := h.evpnMirror.EnsureMirror(evpnmgr.TunnelSpec{
			Interface:  cfg.Name,
			VNI:        cfg.Vxlan.VNI,
			Src:        cfg.Vxlan.Src,
			MTU:        cfg.MTU,
			Pseudowire: pseudowireOnTransport(hctx, cfg.Name),
		}); err != nil {
			return err
		}
	}

	if h.dataplaneState != nil && h.dataplaneState.IsInterfaceConfigured(cfg.Name) {
		return nil
	}

	// PWTransport is derived state (json:"-") that does not survive the
	// config deep copy between load and apply; re-derive it so the
	// tunnel is created with the pseudowire decap next.
	if cfg.Vxlan != nil && !cfg.Vxlan.PWTransport && hctx.Config != nil {
		for _, other := range hctx.Config.Interfaces {
			if other != nil && other.Pseudowire != nil && other.Pseudowire.Transport == cfg.Name {
				cfg.Vxlan.PWTransport = true
				break
			}
		}
	}

	if err := h.southbound.CreateInterface(cfg); err != nil {
		return err
	}

	if cfg.MTU == 0 {
		return h.southbound.SetInterfaceMTU(cfg.Name, interfaces.DefaultMTU)
	}
	return nil
}

func (h *InterfaceHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg := hctx.NewValue.(*interfaces.InterfaceConfig)
	if cfg.Vxlan != nil && cfg.Vxlan.EVPNSignaled() && h.evpnMirror != nil {
		h.evpnMirror.RemoveMirror(cfg.Vxlan.VNI)
	}
	return h.southbound.DeleteInterface(cfg.Name)
}

func (h *InterfaceHandler) PathPattern() paths.Path {
	return paths.Interface
}

func (h *InterfaceHandler) Dependencies() []paths.Path {
	return nil
}

func (h *InterfaceHandler) Callbacks() *conf.Callbacks {
	return nil
}

func (h *InterfaceHandler) Summary() string {
	return "Interface configuration"
}

func (h *InterfaceHandler) Description() string {
	return "Create or configure a top-level network interface."
}

func (h *InterfaceHandler) ValueType() interface{} {
	return &interfaces.InterfaceConfig{}
}
