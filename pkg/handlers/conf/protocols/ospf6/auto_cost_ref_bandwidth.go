package ospf6

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPF6AutoCostRefBandwidthHandler)
}

type OSPF6AutoCostRefBandwidthHandler struct{}

func NewOSPF6AutoCostRefBandwidthHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPF6AutoCostRefBandwidthHandler{}
}

func (h *OSPF6AutoCostRefBandwidthHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, err := conf.ParseUint32(hctx.NewValue)
	if err != nil {
		return fmt.Errorf("invalid auto-cost-reference-bandwidth: %w", err)
	}
	return nil
}

func (h *OSPF6AutoCostRefBandwidthHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6AutoCostRefBandwidthHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6AutoCostRefBandwidthHandler) PathPattern() paths.Path {
	return paths.OSPF6AutoCostRefBandwidth
}

func (h *OSPF6AutoCostRefBandwidthHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPF6Enabled}
}

func (h *OSPF6AutoCostRefBandwidthHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
