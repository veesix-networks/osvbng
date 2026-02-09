package isis

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewISISMaxLSPLifetimeHandler)
}

type ISISMaxLSPLifetimeHandler struct{}

func NewISISMaxLSPLifetimeHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISMaxLSPLifetimeHandler{}
}

func (h *ISISMaxLSPLifetimeHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	val, err := conf.ParseUint32(hctx.NewValue)
	if err != nil {
		return fmt.Errorf("invalid max-lsp-lifetime: %w", err)
	}

	if val < 350 || val > 65535 {
		return fmt.Errorf("max-lsp-lifetime %d out of range (350-65535)", val)
	}

	return nil
}

func (h *ISISMaxLSPLifetimeHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISMaxLSPLifetimeHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISMaxLSPLifetimeHandler) PathPattern() paths.Path {
	return paths.ISISMaxLSPLifetime
}

func (h *ISISMaxLSPLifetimeHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISMaxLSPLifetimeHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
