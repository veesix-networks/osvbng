package ospf

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewOSPFAutoCostRefBandwidthHandler)
}

type OSPFAutoCostRefBandwidthHandler struct{}

func NewOSPFAutoCostRefBandwidthHandler(deps *deps.ConfDeps) conf.Handler {
	return &OSPFAutoCostRefBandwidthHandler{}
}

func (h *OSPFAutoCostRefBandwidthHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, err := conf.ParseUint32(hctx.NewValue)
	if err != nil {
		return fmt.Errorf("invalid auto-cost-reference-bandwidth: %w", err)
	}
	return nil
}

func (h *OSPFAutoCostRefBandwidthHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFAutoCostRefBandwidthHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFAutoCostRefBandwidthHandler) PathPattern() paths.Path {
	return paths.OSPFAutoCostRefBandwidth
}

func (h *OSPFAutoCostRefBandwidthHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.OSPFEnabled}
}

func (h *OSPFAutoCostRefBandwidthHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
