package unicast

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewIPv4VPNLabelHandler)
}

type IPv4VPNLabelHandler struct {
	callbacks *conf.Callbacks
}

func NewIPv4VPNLabelHandler(deps *deps.ConfDeps) conf.Handler {
	return &IPv4VPNLabelHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *IPv4VPNLabelHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNLabelHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNLabelHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv4VPNLabelHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVRFIPv4UnicastLabelVPN
}

func (h *IPv4VPNLabelHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPInstance}
}

func (h *IPv4VPNLabelHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}

func (h *IPv4VPNLabelHandler) Summary() string {
	return "BGP VRF IPv4 VPN label mode"
}

func (h *IPv4VPNLabelHandler) Description() string {
	return "Set the VPN label allocation mode for a VRF IPv4 unicast address family."
}

func (h *IPv4VPNLabelHandler) ValueType() interface{} {
	return ""
}
