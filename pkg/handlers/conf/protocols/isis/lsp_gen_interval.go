package isis

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewISISLSPGenIntervalHandler)
}

type ISISLSPGenIntervalHandler struct{}

func NewISISLSPGenIntervalHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISLSPGenIntervalHandler{}
}

func (h *ISISLSPGenIntervalHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	val, err := conf.ParseUint32(hctx.NewValue)
	if err != nil {
		return fmt.Errorf("invalid lsp-gen-interval: %w", err)
	}

	if val < 1 || val > 120 {
		return fmt.Errorf("lsp-gen-interval %d out of range (1-120)", val)
	}

	return nil
}

func (h *ISISLSPGenIntervalHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISLSPGenIntervalHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISLSPGenIntervalHandler) PathPattern() paths.Path {
	return paths.ISISLSPGenInterval
}

func (h *ISISLSPGenIntervalHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISLSPGenIntervalHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
