package internal

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
	pkgpaths "github.com/veesix-networks/osvbng/pkg/paths"
)

func init() {
	conf.RegisterFactory(NewPuntARPHandler)
	conf.RegisterFactory(NewPuntDHCPv4Handler)
	conf.RegisterFactory(NewPuntDHCPv6Handler)
	conf.RegisterFactory(NewPuntPPPoEHandler)
	conf.RegisterFactory(NewPuntIPv6NDHandler)
}

type PuntHandler struct {
	dataplaneState operations.DataplaneStateReader
	pathPattern    paths.Path
	protocol       uint8
	enableFunc     func(ifName string) error
}

func NewPuntARPHandler(d *deps.ConfDeps) conf.Handler {
	return &PuntHandler{
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InternalPuntARP,
		protocol:       operations.PuntProtoARP,
		enableFunc:     d.Southbound.EnableARPPunt,
	}
}

func NewPuntDHCPv4Handler(d *deps.ConfDeps) conf.Handler {
	return &PuntHandler{
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InternalPuntDHCPv4,
		protocol:       operations.PuntProtoDHCPv4,
		enableFunc:     d.Southbound.EnableDHCPv4Punt,
	}
}

func NewPuntDHCPv6Handler(d *deps.ConfDeps) conf.Handler {
	return &PuntHandler{
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InternalPuntDHCPv6,
		protocol:       operations.PuntProtoDHCPv6,
		enableFunc:     d.Southbound.EnableDHCPv6Punt,
	}
}

func NewPuntPPPoEHandler(d *deps.ConfDeps) conf.Handler {
	return &PuntHandler{
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InternalPuntPPPoE,
		protocol:       operations.PuntProtoPPPoEDisc,
		enableFunc:     d.Southbound.EnablePPPoEPunt,
	}
}

func NewPuntIPv6NDHandler(d *deps.ConfDeps) conf.Handler {
	return &PuntHandler{
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InternalPuntIPv6ND,
		protocol:       operations.PuntProtoIPv6ND,
		enableFunc:     d.Southbound.EnableIPv6NDPunt,
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

	return h.enableFunc(ifName)
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
