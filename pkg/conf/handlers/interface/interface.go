package iface

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/conf/types"
	"github.com/veesix-networks/osvbng/pkg/operations"
)

func init() {
	handlers.RegisterFactory(NewInterfaceHandler)
}

type InterfaceHandler struct {
	dataplane operations.Dataplane
}

func NewInterfaceHandler(daemons *handlers.ConfDeps) handlers.Handler {
	return &InterfaceHandler{dataplane: daemons.Dataplane}
}

func (h *InterfaceHandler) Validate(ctx context.Context, hctx *handlers.HandlerContext) error {
	_, ok := hctx.NewValue.(*types.InterfaceConfig)
	if !ok {
		return fmt.Errorf("expected *types.InterfaceConfig, got %T", hctx.NewValue)
	}
	return nil
}

func (h *InterfaceHandler) Apply(ctx context.Context, hctx *handlers.HandlerContext) error {
	cfg := hctx.NewValue.(*types.InterfaceConfig)
	return h.dataplane.CreateInterface(cfg)
}

func (h *InterfaceHandler) Rollback(ctx context.Context, hctx *handlers.HandlerContext) error {
	cfg := hctx.NewValue.(*types.InterfaceConfig)
	return h.dataplane.DeleteInterface(cfg.Name)
}

func (h *InterfaceHandler) PathPattern() paths.Path {
	return paths.Interface
}

func (h *InterfaceHandler) Dependencies() []paths.Path {
	return nil
}

func (h *InterfaceHandler) Callbacks() *handlers.Callbacks {
	return nil
}
