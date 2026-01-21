package bgp

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewPeerGroupHandler)
}

type PeerGroupHandler struct {
	callbacks *conf.Callbacks
}

func NewPeerGroupHandler(deps *deps.ConfDeps) conf.Handler {
	return &PeerGroupHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *PeerGroupHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(*protocols.BGPPeerGroup)
	if !ok {
		return fmt.Errorf("expected *protocols.BGPPeerGroup, got %T", hctx.NewValue)
	}

	return nil
}

func (h *PeerGroupHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *PeerGroupHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *PeerGroupHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPPeerGroup
}

func (h *PeerGroupHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPInstance}
}

func (h *PeerGroupHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}
