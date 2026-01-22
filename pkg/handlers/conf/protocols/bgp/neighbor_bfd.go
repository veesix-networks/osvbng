package bgp

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewBGPNeighborBFDHandler)
}

type BGPNeighborBFDHandler struct {
}

func NewBGPNeighborBFDHandler(deps *deps.ConfDeps) conf.Handler {
	return &BGPNeighborBFDHandler{}
}

func (h *BGPNeighborBFDHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	if hctx.NewValue == nil {
		return nil
	}

	if _, ok := hctx.NewValue.(bool); !ok {
		return fmt.Errorf("bfd must be a boolean value")
	}

	return nil
}

func (h *BGPNeighborBFDHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *BGPNeighborBFDHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *BGPNeighborBFDHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighborBFD
}

func (h *BGPNeighborBFDHandler) Dependencies() []paths.Path {
	return []paths.Path{
		paths.ProtocolsBGPASN,
		paths.ProtocolsBGPNeighborRemoteAS,
	}
}

func (h *BGPNeighborBFDHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
