package iface

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewInterfaceMTUHandler)
	conf.RegisterFactory(NewSubinterfaceMTUHandler)
}

type MTUHandler struct {
	southbound     southbound.Southbound
	dataplaneState operations.DataplaneStateReader
	pathPattern    paths.Path
	dependencies   []paths.Path
}

func NewInterfaceMTUHandler(d *deps.ConfDeps) conf.Handler {
	return &MTUHandler{
		southbound:     d.Southbound,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InterfaceMTU,
		dependencies:   []paths.Path{paths.Interface},
	}
}

func NewSubinterfaceMTUHandler(d *deps.ConfDeps) conf.Handler {
	return &MTUHandler{
		southbound:     d.Southbound,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InterfaceSubinterfaceMTU,
		dependencies:   []paths.Path{paths.InterfaceSubinterface},
	}
}

func (h *MTUHandler) extractInterfaceName(path string) (string, error) {
	values, err := h.pathPattern.ExtractWildcards(path, 2)
	if err != nil {
		return "", err
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return fmt.Sprintf("%s.%s", values[0], values[1]), nil
}

func (h *MTUHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	var mtu int
	switch v := hctx.NewValue.(type) {
	case int:
		mtu = v
	case int64:
		mtu = int(v)
	case float64:
		mtu = int(v)
	default:
		return fmt.Errorf("MTU must be an integer")
	}

	if mtu < 68 || mtu > 9000 {
		return fmt.Errorf("MTU must be between 68 and 9000")
	}

	return nil
}

func (h *MTUHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName, err := h.extractInterfaceName(hctx.Path)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}

	var mtu int
	switch v := hctx.NewValue.(type) {
	case int:
		mtu = v
	case int64:
		mtu = int(v)
	case float64:
		mtu = int(v)
	}

	if h.dataplaneState != nil {
		currentMTU := h.dataplaneState.GetInterfaceMTU(ifName)
		if currentMTU == uint32(mtu) {
			return nil
		}
	}

	return h.southbound.SetInterfaceMTU(ifName, mtu)
}

func (h *MTUHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName, err := h.extractInterfaceName(hctx.Path)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}

	if hctx.OldValue == nil {
		return nil
	}

	var oldMTU int
	switch v := hctx.OldValue.(type) {
	case int:
		oldMTU = v
	case int64:
		oldMTU = int(v)
	case float64:
		oldMTU = int(v)
	}

	return h.southbound.SetInterfaceMTU(ifName, oldMTU)
}

func (h *MTUHandler) PathPattern() paths.Path {
	return h.pathPattern
}

func (h *MTUHandler) Dependencies() []paths.Path {
	return h.dependencies
}

func (h *MTUHandler) Callbacks() *conf.Callbacks {
	return nil
}
