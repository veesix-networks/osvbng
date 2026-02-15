package unicast

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewIPv6VPNLabelHandler)
}

type IPv6VPNLabelHandler struct {
	callbacks *conf.Callbacks
}

func NewIPv6VPNLabelHandler(deps *deps.ConfDeps) conf.Handler {
	return &IPv6VPNLabelHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *IPv6VPNLabelHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNLabelHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNLabelHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNLabelHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVRFIPv6UnicastLabelVPN
}

func (h *IPv6VPNLabelHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPInstance}
}

func (h *IPv6VPNLabelHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}
