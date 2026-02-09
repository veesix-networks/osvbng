package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPFDefaultInformationHandler)
}

type OSPFDefaultInformationHandler struct{}

func NewOSPFDefaultInformationHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPFDefaultInformationHandler{}
}

func (h *OSPFDefaultInformationHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*protocols.OSPFDefaultInfo)
	if !ok {
		return fmt.Errorf("expected *protocols.OSPFDefaultInfo, got %T", hctx.NewValue)
	}

	if cfg.MetricType != 0 && cfg.MetricType != 1 && cfg.MetricType != 2 {
		return fmt.Errorf("invalid metric-type %d: must be 1 or 2", cfg.MetricType)
	}

	return nil
}

func (h *OSPFDefaultInformationHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFDefaultInformationHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFDefaultInformationHandler) PathPattern() paths.Path {
	return paths.OSPFDefaultInformation
}

func (h *OSPFDefaultInformationHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPFEnabled}
}

func (h *OSPFDefaultInformationHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
