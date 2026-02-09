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
	conf.RegisterFactory(NewISISMetricStyleHandler)
}

type ISISMetricStyleHandler struct{}

func NewISISMetricStyleHandler(deps *deps.ConfDeps) conf.Handler {
	return &ISISMetricStyleHandler{}
}

func (h *ISISMetricStyleHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	val, ok := hctx.NewValue.(protocols.ISISMetricStyle)
	if !ok {
		return fmt.Errorf("expected protocols.ISISMetricStyle, got %T", hctx.NewValue)
	}

	if val != "" && !val.Valid() {
		return fmt.Errorf("invalid metric-style %q", val)
	}

	return nil
}

func (h *ISISMetricStyleHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISMetricStyleHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *ISISMetricStyleHandler) PathPattern() paths.Path {
	return paths.ISISMetricStyle
}

func (h *ISISMetricStyleHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ISISEnabled}
}

func (h *ISISMetricStyleHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
