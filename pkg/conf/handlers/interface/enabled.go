package iface

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
)

func init() {
	handlers.RegisterFactory(NewEnabledHandler)
}

type EnabledHandler struct {
	dataplane operations.Dataplane
}

func NewEnabledHandler(daemons *handlers.ConfDeps) handlers.Handler {
	return &EnabledHandler{dataplane: daemons.Dataplane}
}

func (h *EnabledHandler) Validate(ctx context.Context, hctx *handlers.HandlerContext) error {
	_, ok := hctx.NewValue.(bool)
	if !ok {
		return fmt.Errorf("enabled must be a boolean")
	}
	return nil
}

func (h *EnabledHandler) Apply(ctx context.Context, hctx *handlers.HandlerContext) error {
	ifName := handlers.ExtractInterfaceName(hctx.Path)
	enabled := hctx.NewValue.(bool)

	return h.dataplane.SetInterfaceEnabled(ifName, enabled)
}

func (h *EnabledHandler) Rollback(ctx context.Context, hctx *handlers.HandlerContext) error {
	ifName := handlers.ExtractInterfaceName(hctx.Path)

	if hctx.OldValue == nil {
		return nil
	}

	oldEnabled := hctx.OldValue.(bool)
	return h.dataplane.SetInterfaceEnabled(ifName, oldEnabled)
}

func (h *EnabledHandler) PathPattern() paths.Path {
	return paths.InterfaceEnabled
}

func (h *EnabledHandler) Dependencies() []paths.Path {
	return []paths.Path{"interfaces.*"}
}

func (h *EnabledHandler) Callbacks() *handlers.Callbacks {
	return nil
}
