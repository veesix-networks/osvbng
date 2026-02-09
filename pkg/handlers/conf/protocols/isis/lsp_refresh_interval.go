package isis

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewISISLSPRefreshIntervalHandler)
}

type ISISLSPRefreshIntervalHandler struct{}

func NewISISLSPRefreshIntervalHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISLSPRefreshIntervalHandler{}
}

func (h *ISISLSPRefreshIntervalHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	val, err := conf.ParseUint32(hctx.NewValue)
	if err != nil {
		return fmt.Errorf("invalid lsp-refresh-interval: %w", err)
	}

	if val < 1 || val > 65235 {
		return fmt.Errorf("lsp-refresh-interval %d out of range (1-65235)", val)
	}

	return nil
}

func (h *ISISLSPRefreshIntervalHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISLSPRefreshIntervalHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISLSPRefreshIntervalHandler) PathPattern() paths.Path {
	return paths.ISISLSPRefreshInterval
}

func (h *ISISLSPRefreshIntervalHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISLSPRefreshIntervalHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
