package iface

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/deps"
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
}

func NewInterfaceHandler(daemons *deps.ConfDeps) conf.Handler {
	return &InterfaceHandler{
		southbound:     daemons.Southbound,
		dataplaneState: daemons.DataplaneState,
	}
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

	if h.dataplaneState != nil && h.dataplaneState.IsInterfaceConfigured(cfg.Name) {
		return nil
	}

	return h.southbound.CreateInterface(cfg)
}

func (h *InterfaceHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg := hctx.NewValue.(*interfaces.InterfaceConfig)
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
