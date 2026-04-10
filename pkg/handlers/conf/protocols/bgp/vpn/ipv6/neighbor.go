package ipv6

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewIPv6VPNNeighborHandler)
}

type IPv6VPNNeighborHandler struct {
	callbacks *conf.Callbacks
}

func NewIPv6VPNNeighborHandler(deps *deps.ConfDeps) conf.Handler {
	return &IPv6VPNNeighborHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *IPv6VPNNeighborHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNNeighborHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNNeighborHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNNeighborHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv6VPNNeighbor
}

func (h *IPv6VPNNeighborHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPIPv6VPN}
}

func (h *IPv6VPNNeighborHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}

func (h *IPv6VPNNeighborHandler) Summary() string {
	return "BGP VPNv6 neighbor"
}

func (h *IPv6VPNNeighborHandler) Description() string {
	return "Configure a BGP VPNv6 neighbor with send-community settings."
}

func (h *IPv6VPNNeighborHandler) ValueType() interface{} {
	return false
}
