package isis

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewISISAreaPasswordHandler)
}

type ISISAreaPasswordHandler struct{}

func NewISISAreaPasswordHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISAreaPasswordHandler{}
}

func (h *ISISAreaPasswordHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if _, ok := hctx.NewValue.(string); !ok {
		return fmt.Errorf("expected string, got %T", hctx.NewValue)
	}
	return nil
}

func (h *ISISAreaPasswordHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISAreaPasswordHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISAreaPasswordHandler) PathPattern() paths.Path {
	return paths.ISISAreaPassword
}

func (h *ISISAreaPasswordHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISAreaPasswordHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
