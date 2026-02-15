package ipv4

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewIPv4VPNHandler)
}

type IPv4VPNHandler struct {
	callbacks *conf.Callbacks
}

func NewIPv4VPNHandler(deps *deps.ConfDeps) conf.Handler {
	return &IPv4VPNHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *IPv4VPNHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv4VPN
}

func (h *IPv4VPNHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPInstance}
}

func (h *IPv4VPNHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}
