package ipv6

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewIPv6VPNHandler)
}

type IPv6VPNHandler struct {
	callbacks *conf.Callbacks
}

func NewIPv6VPNHandler(deps *deps.ConfDeps) conf.Handler {
	return &IPv6VPNHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *IPv6VPNHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv6VPN
}

func (h *IPv6VPNHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPInstance}
}

func (h *IPv6VPNHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}
