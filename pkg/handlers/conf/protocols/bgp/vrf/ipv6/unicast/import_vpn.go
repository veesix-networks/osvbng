package unicast

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewIPv6VPNImportHandler)
}

type IPv6VPNImportHandler struct {
	callbacks *conf.Callbacks
}

func NewIPv6VPNImportHandler(deps *deps.ConfDeps) conf.Handler {
	return &IPv6VPNImportHandler{
		callbacks: &conf.Callbacks{
			OnAfterApply: func(hctx *conf.HandlerContext, err error) {
				if err == nil {
					hctx.MarkFRRReloadNeeded()
				}
			},
		},
	}
}

func (h *IPv6VPNImportHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNImportHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNImportHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6VPNImportHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPVRFIPv6UnicastImportVPN
}

func (h *IPv6VPNImportHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.ProtocolsBGPInstance}
}

func (h *IPv6VPNImportHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}
