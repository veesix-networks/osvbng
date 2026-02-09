package isis

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewISISSPFIntervalHandler)
}

type ISISSPFIntervalHandler struct{}

func NewISISSPFIntervalHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISSPFIntervalHandler{}
}

func (h *ISISSPFIntervalHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	val, err := conf.ParseUint32(hctx.NewValue)
	if err != nil {
		return fmt.Errorf("invalid spf-interval: %w", err)
	}

	if val < 1 || val > 120 {
		return fmt.Errorf("spf-interval %d out of range (1-120)", val)
	}

	return nil
}

func (h *ISISSPFIntervalHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISSPFIntervalHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISSPFIntervalHandler) PathPattern() paths.Path {
	return paths.ISISSPFInterval
}

func (h *ISISSPFIntervalHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISSPFIntervalHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
