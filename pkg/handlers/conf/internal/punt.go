package internal

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
	pkgpaths "github.com/veesix-networks/osvbng/pkg/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewPuntARPHandler)
	conf.RegisterFactory(NewPuntDHCPv4Handler)
	conf.RegisterFactory(NewPuntDHCPv6Handler)
	conf.RegisterFactory(NewPuntPPPoEHandler)
}

type PuntHandler struct {
	southbound     *southbound.VPP
	dataplaneState operations.DataplaneStateReader
	pathPattern    paths.Path
	protocol       uint8
	enableFunc     func(sb *southbound.VPP, ifName, socketPath string) error
}

func NewPuntARPHandler(d *deps.ConfDeps) conf.Handler {
	return &PuntHandler{
		southbound:     d.Southbound,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InternalPuntARP,
		protocol:       operations.PuntProtoARP,
		enableFunc:     (*southbound.VPP).EnableARPPunt,
	}
}

func NewPuntDHCPv4Handler(d *deps.ConfDeps) conf.Handler {
	return &PuntHandler{
		southbound:     d.Southbound,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InternalPuntDHCPv4,
		protocol:       operations.PuntProtoDHCPv4,
		enableFunc:     (*southbound.VPP).EnableDHCPv4Punt,
	}
}

func NewPuntDHCPv6Handler(d *deps.ConfDeps) conf.Handler {
	return &PuntHandler{
		southbound:     d.Southbound,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InternalPuntDHCPv6,
		protocol:       operations.PuntProtoDHCPv6,
		enableFunc:     (*southbound.VPP).EnableDHCPv6Punt,
	}
}

func NewPuntPPPoEHandler(d *deps.ConfDeps) conf.Handler {
	return &PuntHandler{
		southbound:     d.Southbound,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InternalPuntPPPoE,
		protocol:       operations.PuntProtoPPPoEDisc,
		enableFunc:     (*southbound.VPP).EnablePPPoEPunt,
	}
}

func (h *PuntHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(*operations.PuntConfig)
	if !ok {
		return fmt.Errorf("expected *operations.PuntConfig, got %T", hctx.NewValue)
	}
	return nil
}

func (h *PuntHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg := hctx.NewValue.(*operations.PuntConfig)
	if !cfg.Enabled {
		return nil
	}

	values, err := h.pathPattern.ExtractWildcards(hctx.Path, 1)
	if err != nil {
		return fmt.Errorf("extract interface from path: %w", err)
	}
	ifName := pkgpaths.DecodeInterfaceName(values[0])

	if h.dataplaneState != nil {
		ifState := h.dataplaneState.GetInterfaceByName(ifName)
		if ifState != nil && h.dataplaneState.IsPuntEnabled(ifState.SwIfIndex, h.protocol) {
			return nil
		}
	}

	return h.enableFunc(h.southbound, ifName, cfg.SocketPath)
}

func (h *PuntHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *PuntHandler) PathPattern() paths.Path {
	return h.pathPattern
}

func (h *PuntHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.InterfaceSubinterface}
}

func (h *PuntHandler) Callbacks() *conf.Callbacks {
	return nil
}
