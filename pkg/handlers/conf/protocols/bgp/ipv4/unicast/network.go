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

type NetworkHandler struct {
	callbacks *conf.Callbacks
}

func NewNetworkHandler(deps *deps.ConfDeps) conf.Handler {
	return &NetworkHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *NetworkHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *NetworkHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *NetworkHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *NetworkHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPIPv4UnicastNetwork
}

func (h *NetworkHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPInstance}
}

func (h *NetworkHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}
