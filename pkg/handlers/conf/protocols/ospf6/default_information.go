package ospf6

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPF6DefaultInformationHandler)
}

type OSPF6DefaultInformationHandler struct{}

func NewOSPF6DefaultInformationHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPF6DefaultInformationHandler{}
}

func (h *OSPF6DefaultInformationHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*protocols.OSPF6DefaultInfo)
	if !ok {
		return fmt.Errorf("expected *protocols.OSPF6DefaultInfo, got %T", hctx.NewValue)
	}

	if cfg.MetricType != 0 && cfg.MetricType != 1 && cfg.MetricType != 2 {
		return fmt.Errorf("invalid metric-type %d: must be 1 or 2", cfg.MetricType)
	}

	return nil
}

func (h *OSPF6DefaultInformationHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6DefaultInformationHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6DefaultInformationHandler) PathPattern() paths.Path {
	return paths.OSPF6DefaultInformation
}

func (h *OSPF6DefaultInformationHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPF6Enabled}
}

func (h *OSPF6DefaultInformationHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
