package isis

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewISISRedistributeHandler)
}

type ISISRedistributeHandler struct{}

func NewISISRedistributeHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISRedistributeHandler{}
}

func (h *ISISRedistributeHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(*protocols.ISISRedistribute)
	if !ok {
		return fmt.Errorf("expected *protocols.ISISRedistribute, got %T", hctx.NewValue)
	}
	return nil
}

func (h *ISISRedistributeHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISRedistributeHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISRedistributeHandler) PathPattern() paths.Path {
	return paths.ISISRedistribute
}

func (h *ISISRedistributeHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISRedistributeHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
