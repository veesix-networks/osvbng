package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPFDefaultMetricHandler)
}

type OSPFDefaultMetricHandler struct{}

func NewOSPFDefaultMetricHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPFDefaultMetricHandler{}
}

func (h *OSPFDefaultMetricHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, err := conf.ParseUint32(hctx.NewValue)
	if err != nil {
		return fmt.Errorf("invalid default-metric: %w", err)
	}
	return nil
}

func (h *OSPFDefaultMetricHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFDefaultMetricHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFDefaultMetricHandler) PathPattern() paths.Path {
	return paths.OSPFDefaultMetric
}

func (h *OSPFDefaultMetricHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPFEnabled}
}

func (h *OSPFDefaultMetricHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
