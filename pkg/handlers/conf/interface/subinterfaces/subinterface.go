package subinterfaces

import (
	"context"
	"fmt"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewSubinterfaceHandler)
}

type SubinterfaceHandler struct {
	southbound     *southbound.VPP
	dataplaneState operations.DataplaneStateReader
}

func NewSubinterfaceHandler(d *deps.ConfDeps) conf.Handler {
	return &SubinterfaceHandler{
		southbound:     d.Southbound,
		dataplaneState: d.DataplaneState,
	}
}

func (h *SubinterfaceHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(*interfaces.SubinterfaceConfig)
	if !ok {
		return fmt.Errorf("expected *interfaces.SubinterfaceConfig, got %T", hctx.NewValue)
	}
	return nil
}

func (h *SubinterfaceHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	values, err := paths.InterfaceSubinterface.ExtractWildcards(hctx.Path, 2)
	if err != nil {
		return fmt.Errorf("extract values from path: %w", err)
	}

	parentIf := values[0]
	subIfID, err := strconv.ParseUint(values[1], 10, 16)
	if err != nil {
		return fmt.Errorf("parse subinterface id: %w", err)
	}

	subIfName := fmt.Sprintf("%s.%d", parentIf, subIfID)
	if h.dataplaneState != nil && h.dataplaneState.IsInterfaceConfigured(subIfName) {
		return nil
	}

	return h.southbound.CreateSVLAN(parentIf, uint16(subIfID), nil, nil)
}

func (h *SubinterfaceHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *SubinterfaceHandler) PathPattern() paths.Path {
	return paths.InterfaceSubinterface
}

func (h *SubinterfaceHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.Interface}
}

func (h *SubinterfaceHandler) Callbacks() *conf.Callbacks {
	return nil
}
