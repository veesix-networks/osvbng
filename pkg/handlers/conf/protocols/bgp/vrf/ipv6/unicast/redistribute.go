package unicast

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewRedistributeHandler)
}

type RedistributeHandler struct {
	callbacks *conf.Callbacks
}

func NewRedistributeHandler(deps *deps.ConfDeps) conf.Handler {
	return &RedistributeHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *RedistributeHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(*protocols.BGPRedistribute)
	if !ok {
		return fmt.Errorf("expected *protocols.BGPRedistribute, got %T", hctx.NewValue)
	}

	return nil
}

func (h *RedistributeHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *RedistributeHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *RedistributeHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVRFIPv6UnicastRedistribute
}

func (h *RedistributeHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPInstance}
}

func (h *RedistributeHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}
