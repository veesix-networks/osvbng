package unicast

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewIPv6VPNExportHandler)
}

type IPv6VPNExportHandler struct {
	callbacks *conf.Callbacks
}

func NewIPv6VPNExportHandler(deps *deps.ConfDeps) conf.Handler {
	return &IPv6VPNExportHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *IPv6VPNExportHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNExportHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNExportHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNExportHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVRFIPv6UnicastExportVPN
}

func (h *IPv6VPNExportHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPInstance}
}

func (h *IPv6VPNExportHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}

func (h *IPv6VPNExportHandler) Summary() string {
	return "BGP VRF IPv6 VPN export"
}

func (h *IPv6VPNExportHandler) Description() string {
	return "Enable VPN route export for a VRF IPv6 unicast address family."
}

func (h *IPv6VPNExportHandler) ValueType() interface{} {
	return false
}
