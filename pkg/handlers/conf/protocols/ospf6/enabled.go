package ospf6

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPF6EnabledHandler)
}

type OSPF6EnabledHandler struct{}

func NewOSPF6EnabledHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPF6EnabledHandler{}
}

func (h *OSPF6EnabledHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if _, ok := hctx.NewValue.(bool); !ok {
		return fmt.Errorf("expected bool, got %T", hctx.NewValue)
	}
	return nil
}

func (h *OSPF6EnabledHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6EnabledHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6EnabledHandler) PathPattern() paths.Path {
	return paths.OSPF6Enabled
}

func (h *OSPF6EnabledHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6EnabledHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
