package iface

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
)

func init() {
	conf.RegisterFactory(NewEnabledHandler)
}

type EnabledHandler struct {
	dataplane operations.Dataplane
}

func NewEnabledHandler(daemons *deps.ConfDeps) conf.Handler {
	return &EnabledHandler{dataplane: daemons.Dataplane}
}

func (h *EnabledHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(bool)
	if !ok {
		return fmt.Errorf("enabled must be a boolean")
	}
	return nil
}

func (h *EnabledHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName := conf.ExtractInterfaceName(hctx.Path)
	enabled := hctx.NewValue.(bool)

	return h.dataplane.SetInterfaceEnabled(ifName, enabled)
}

func (h *EnabledHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName := conf.ExtractInterfaceName(hctx.Path)

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
	return []paths.Path{paths.Interface}
}

func (h *EnabledHandler) Callbacks() *conf.Callbacks {
	return nil
}
