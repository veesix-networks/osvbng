package unicast

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewIPv4VPNImportHandler)
}

type IPv4VPNImportHandler struct {
	callbacks *conf.Callbacks
}

func NewIPv4VPNImportHandler(deps *deps.ConfDeps) conf.Handler {
	return &IPv4VPNImportHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *IPv4VPNImportHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNImportHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNImportHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNImportHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVRFIPv4UnicastImportVPN
}

func (h *IPv4VPNImportHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPInstance}
}

func (h *IPv4VPNImportHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}

func (h *IPv4VPNImportHandler) Summary() string {
	return "BGP VRF IPv4 VPN import"
}

func (h *IPv4VPNImportHandler) Description() string {
	return "Enable VPN route import for a VRF IPv4 unicast address family."
}

func (h *IPv4VPNImportHandler) ValueType() interface{} {
	return false
}
