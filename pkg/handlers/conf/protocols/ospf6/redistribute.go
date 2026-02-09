package ospf6

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPF6RedistributeHandler)
}

type OSPF6RedistributeHandler struct{}

func NewOSPF6RedistributeHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPF6RedistributeHandler{}
}

func (h *OSPF6RedistributeHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(*protocols.OSPF6Redistribute)
	if !ok {
		return fmt.Errorf("expected *protocols.OSPF6Redistribute, got %T", hctx.NewValue)
	}
	return nil
}

func (h *OSPF6RedistributeHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6RedistributeHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6RedistributeHandler) PathPattern() paths.Path {
	return paths.OSPF6Redistribute
}

func (h *OSPF6RedistributeHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPF6Enabled}
}

func (h *OSPF6RedistributeHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
