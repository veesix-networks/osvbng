package bgp

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewBGPNeighborDescriptionHandler)
}

type BGPNeighborDescriptionHandler struct {
}

func NewBGPNeighborDescriptionHandler(deps *deps.ConfDeps) conf.Handler {
	return &BGPNeighborDescriptionHandler{}
}

func (h *BGPNeighborDescriptionHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *BGPNeighborDescriptionHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *BGPNeighborDescriptionHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *BGPNeighborDescriptionHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighborDescription
}

func (h *BGPNeighborDescriptionHandler) Dependencies() []paths.Path {
	return []paths.Path{
		paths.ProtocolsBGPASN,
		paths.ProtocolsBGPNeighborRemoteAS,
	}
}

func (h *BGPNeighborDescriptionHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
