package ospf6

import (
	"context"
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPF6RouterIDHandler)
}

type OSPF6RouterIDHandler struct{}

func NewOSPF6RouterIDHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPF6RouterIDHandler{}
}

func (h *OSPF6RouterIDHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	val, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("expected string, got %T", hctx.NewValue)
	}

	if val != "" && net.ParseIP(val) == nil {
		return fmt.Errorf("invalid router-id %q: must be A.B.C.D format", val)
	}

	return nil
}

func (h *OSPF6RouterIDHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6RouterIDHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6RouterIDHandler) PathPattern() paths.Path {
	return paths.OSPF6RouterID
}

func (h *OSPF6RouterIDHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPF6Enabled}
}

func (h *OSPF6RouterIDHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
