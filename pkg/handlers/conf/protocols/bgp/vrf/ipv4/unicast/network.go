package unicast

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewNetworkHandler)
}

type VRFNetworkHandler struct {
	callbacks *conf.Callbacks
}

func NewNetworkHandler(deps *deps.ConfDeps) conf.Handler {
	return &VRFNetworkHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *VRFNetworkHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *VRFNetworkHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *VRFNetworkHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *VRFNetworkHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVRFIPv4UnicastNetwork
}

func (h *VRFNetworkHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPInstance}
}

func (h *VRFNetworkHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}
