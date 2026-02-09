package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPFRedistributeHandler)
}

type OSPFRedistributeHandler struct{}

func NewOSPFRedistributeHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPFRedistributeHandler{}
}

func (h *OSPFRedistributeHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(*protocols.OSPFRedistribute)
	if !ok {
		return fmt.Errorf("expected *protocols.OSPFRedistribute, got %T", hctx.NewValue)
	}
	return nil
}

func (h *OSPFRedistributeHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFRedistributeHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFRedistributeHandler) PathPattern() paths.Path {
	return paths.OSPFRedistribute
}

func (h *OSPFRedistributeHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPFEnabled}
}

func (h *OSPFRedistributeHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
