package bgp

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewBGPNeighborRemoteASHandler)
}

type BGPNeighborRemoteASHandler struct {
}

func NewBGPNeighborRemoteASHandler(deps *deps.ConfDeps) conf.Handler {
	return &BGPNeighborRemoteASHandler{}
}

func (h *BGPNeighborRemoteASHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	asn, err := conf.ParseUint32(hctx.NewValue)
	if err != nil {
		return fmt.Errorf("invalid remote-as: %w", err)
	}

	if asn == 0 {
		return fmt.Errorf("remote-as cannot be 0")
	}

	return nil
}

func (h *BGPNeighborRemoteASHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *BGPNeighborRemoteASHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *BGPNeighborRemoteASHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPNeighborRemoteAS
}

func (h *BGPNeighborRemoteASHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPASN}
}

func (h *BGPNeighborRemoteASHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
