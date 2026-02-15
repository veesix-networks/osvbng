package ipv4

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewIPv4VPNNeighborHandler)
}

type IPv4VPNNeighborHandler struct {
	callbacks *conf.Callbacks
}

func NewIPv4VPNNeighborHandler(deps *deps.ConfDeps) conf.Handler {
	return &IPv4VPNNeighborHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *IPv4VPNNeighborHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNNeighborHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNNeighborHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNNeighborHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv4VPNNeighbor
}

func (h *IPv4VPNNeighborHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPIPv4VPN}
}

func (h *IPv4VPNNeighborHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}
