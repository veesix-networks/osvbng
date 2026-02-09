package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPFEnabledHandler)
}

type OSPFEnabledHandler struct{}

func NewOSPFEnabledHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPFEnabledHandler{}
}

func (h *OSPFEnabledHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if _, ok := hctx.NewValue.(bool); !ok {
		return fmt.Errorf("expected bool, got %T", hctx.NewValue)
	}
	return nil
}

func (h *OSPFEnabledHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFEnabledHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFEnabledHandler) PathPattern() paths.Path {
	return paths.OSPFEnabled
}

func (h *OSPFEnabledHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPFEnabledHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
