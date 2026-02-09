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
	conf.RegisterFactory(NewISISDefaultInformationHandler)
}

type ISISDefaultInformationHandler struct{}

func NewISISDefaultInformationHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISDefaultInformationHandler{}
}

func (h *ISISDefaultInformationHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(*protocols.ISISDefaultInfo)
	if !ok {
		return fmt.Errorf("expected *protocols.ISISDefaultInfo, got %T", hctx.NewValue)
	}
	return nil
}

func (h *ISISDefaultInformationHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISDefaultInformationHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISDefaultInformationHandler) PathPattern() paths.Path {
	return paths.ISISDefaultInformation
}

func (h *ISISDefaultInformationHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISDefaultInformationHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
