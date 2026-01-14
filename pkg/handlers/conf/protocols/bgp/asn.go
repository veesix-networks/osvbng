package bgp

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewBGPASNHandler)
}

type BGPASNHandler struct {
	callbacks *conf.Callbacks
}

func NewBGPASNHandler(deps *deps.ConfDeps) conf.Handler {
	h := &BGPASNHandler{}

	h.callbacks = &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}

	return h
}

func (h *BGPASNHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	asn, ok := hctx.NewValue.(uint32)
	if !ok {
		return fmt.Errorf("expected uint32 for ASN, got %T", hctx.NewValue)
	}

	if asn == 0 {
		return fmt.Errorf("BGP ASN cannot be 0")
	}

	return nil
}

func (h *BGPASNHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *BGPASNHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *BGPASNHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPASN
}

func (h *BGPASNHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPASNHandler) Callbacks() *conf.Callbacks {
	return h.callbacks
}
