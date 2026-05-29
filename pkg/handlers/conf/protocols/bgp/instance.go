package bgp

import (
	"context"
	"fmt"

	routingcomp "github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
)

func init() {
	conf.RegisterFactory(NewBGPInstanceHandler)
}

type BGPInstanceHandler struct {
	routing *routingcomp.Component
}

func NewBGPInstanceHandler(deps *deps.ConfDeps) conf.Handler {
	return &BGPInstanceHandler{
		routing: deps.Routing,
	}
}

func (h *BGPInstanceHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*protocols.BGPConfig)
	if !ok {
		return fmt.Errorf("expected *protocols.BGPConfig, got %T", hctx.NewValue)
	}

	if cfg.RouterID == "" {
		return fmt.Errorf("router-id is required for BGP instance")
	}

	return nil
}

func (h *BGPInstanceHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*protocols.BGPConfig)
	if !ok {
		return fmt.Errorf("expected *protocols.BGPConfig, got %T", hctx.NewValue)
	}
	return h.routing.ConfigureBGP(cfg.ASN, cfg.RouterID)
}

func (h *BGPInstanceHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	// Rolling back a freshly-added instance has no OldValue; undo it using the
	// value that was applied. Without this guard the nil type assertion panics
	// and crashes the daemon mid-rollback, masking the error that triggered it.
	cfg, ok := hctx.OldValue.(*protocols.BGPConfig)
	if !ok {
		cfg, ok = hctx.NewValue.(*protocols.BGPConfig)
	}
	if !ok {
		return nil
	}
	return h.routing.RemoveBGP(cfg.ASN)
}

func (h *BGPInstanceHandler) PathPattern() paths.Path {
	return paths.ProtocolsBGPInstance
}

func (h *BGPInstanceHandler) Dependencies() []paths.Path {
	return nil
}

func (h *BGPInstanceHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}

func (h *BGPInstanceHandler) Summary() string {
	return "BGP instance configuration"
}

func (h *BGPInstanceHandler) Description() string {
	return "Configure the BGP routing protocol instance."
}

func (h *BGPInstanceHandler) ValueType() interface{} {
	return &protocols.BGPConfig{}
}
