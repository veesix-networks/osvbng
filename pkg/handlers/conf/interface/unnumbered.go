package iface

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/southbound/vpp"
)

func init() {
	conf.RegisterFactory(NewInterfaceUnnumberedHandler)
	conf.RegisterFactory(NewSubinterfaceUnnumberedHandler)
}

type UnnumberedHandler struct {
	southbound     *vpp.VPP
	dataplaneState operations.DataplaneStateReader
	pathPattern    paths.Path
	dependencies   []paths.Path
}

func NewInterfaceUnnumberedHandler(d *deps.ConfDeps) conf.Handler {
	return &UnnumberedHandler{
		southbound:     d.Southbound,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InterfaceUnnumbered,
		dependencies:   []paths.Path{paths.Interface},
	}
}

func NewSubinterfaceUnnumberedHandler(d *deps.ConfDeps) conf.Handler {
	return &UnnumberedHandler{
		southbound:     d.Southbound,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InterfaceSubinterfaceUnnumbered,
		dependencies:   []paths.Path{paths.InterfaceSubinterface},
	}
}

func (h *UnnumberedHandler) extractInterfaceName(path string) (string, error) {
	values, err := h.pathPattern.ExtractWildcards(path, 2)
	if err != nil {
		return "", err
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return fmt.Sprintf("%s.%s", values[0], values[1]), nil
}

func (h *UnnumberedHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("expected string (loopback name), got %T", hctx.NewValue)
	}
	return nil
}

func (h *UnnumberedHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	loopback := hctx.NewValue.(string)
	if loopback == "" {
		return nil
	}

	ifName, err := h.extractInterfaceName(hctx.Path)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}

	if h.dataplaneState != nil {
		ifState := h.dataplaneState.GetInterfaceByName(ifName)
		if ifState != nil && h.dataplaneState.IsUnnumberedConfigured(ifState.SwIfIndex) {
			return nil
		}
	}

	return h.southbound.SetUnnumbered(ifName, loopback)
}

func (h *UnnumberedHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *UnnumberedHandler) PathPattern() paths.Path {
	return h.pathPattern
}

func (h *UnnumberedHandler) Dependencies() []paths.Path {
	return h.dependencies
}

func (h *UnnumberedHandler) Callbacks() *conf.Callbacks {
	return nil
}
