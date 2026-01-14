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
	conf.RegisterFactory(NewMTUHandler)
}

type MTUHandler struct {
	dataplane operations.Dataplane
}

func NewMTUHandler(daemons *deps.ConfDeps) conf.Handler {
	return &MTUHandler{dataplane: daemons.Dataplane}
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
	ifName := conf.ExtractInterfaceName(hctx.Path)

	var mtu int
	switch v := hctx.NewValue.(type) {
	case int:
		mtu = v
	case int64:
		mtu = int(v)
	case float64:
		mtu = int(v)
	}

	return h.dataplane.SetInterfaceMTU(ifName, mtu)
}

func (h *MTUHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName := conf.ExtractInterfaceName(hctx.Path)

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

	return h.dataplane.SetInterfaceMTU(ifName, oldMTU)
}

func (h *MTUHandler) PathPattern() paths.Path {
	return paths.InterfaceMTU
}

func (h *MTUHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.Interface}
}

func (h *MTUHandler) Callbacks() *conf.Callbacks {
	return nil
}
