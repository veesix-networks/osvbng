package unicast

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewIPv4VPNExportHandler)
}

type IPv4VPNExportHandler struct {
	callbacks *conf.Callbacks
}

func NewIPv4VPNExportHandler(deps *deps.ConfDeps) conf.Handler {
	return &IPv4VPNExportHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *IPv4VPNExportHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNExportHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNExportHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNExportHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVRFIPv4UnicastExportVPN
}

func (h *IPv4VPNExportHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPInstance}
}

func (h *IPv4VPNExportHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}

func (h *IPv4VPNExportHandler) Summary() string {
	return "BGP VRF IPv4 VPN export"
}

func (h *IPv4VPNExportHandler) Description() string {
	return "Enable VPN route export for a VRF IPv4 unicast address family."
}

func (h *IPv4VPNExportHandler) ValueType() interface{} {
	return false
}
