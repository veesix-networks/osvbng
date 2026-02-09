package isis

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewISISLSPMTUHandler)
}

type ISISLSPMTUHandler struct{}

func NewISISLSPMTUHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISLSPMTUHandler{}
}

func (h *ISISLSPMTUHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	val, err := conf.ParseUint32(hctx.NewValue)
	if err != nil {
		return fmt.Errorf("invalid lsp-mtu: %w", err)
	}

	if val < 128 || val > 4352 {
		return fmt.Errorf("lsp-mtu %d out of range (128-4352)", val)
	}

	return nil
}

func (h *ISISLSPMTUHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISLSPMTUHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISLSPMTUHandler) PathPattern() paths.Path {
	return paths.ISISLSPMTU
}

func (h *ISISLSPMTUHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISLSPMTUHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
