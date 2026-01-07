package bgp

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/conf/handlers"
	"github.com/veesix-networks/osvbng/pkg/conf/paths"
)

func init() {
	handlers.RegisterFactory(NewBGPASNHandler)
}

type BGPASNHandler struct {
	callbacks *handlers.Callbacks
}

func NewBGPASNHandler(deps *handlers.ConfDeps) handlers.Handler {
	h := &BGPASNHandler{}

	h.callbacks = &handlers.Callbacks{
		OnAfterApply: func(hctx *handlers.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}

	return h
}

func (h *BGPASNHandler) Validate(ctx context.Context, hctx *handlers.HandlerContext) error {
	asn, ok := hctx.NewValue.(uint32)
	if !ok {
		return fmt.Errorf("expected uint32 for ASN, got %T", hctx.NewValue)
	}

	if asn == 0 {
		return fmt.Errorf("BGP ASN cannot be 0")
	}

	return nil
}

func (h *BGPASNHandler) Apply(ctx context.Context, hctx *handlers.HandlerContext) error {
	return nil
}

func (h *BGPASNHandler) Rollback(ctx context.Context, hctx *handlers.HandlerContext) error {
	return nil
}

func (h *BGPASNHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPASN
}

func (h *BGPASNHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPASNHandler) Callbacks() *handlers.Callbacks {
	return h.callbacks
}
