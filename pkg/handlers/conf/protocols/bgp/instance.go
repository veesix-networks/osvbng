package bgp

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"
	"fmt"

	routingcomp "github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/config"
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
	cfg, ok := hctx.NewValue.(*config.BGPConfig)
	if !ok {
		return fmt.Errorf("expected *config.BGPConfig, got %T", hctx.NewValue)
	}

	if cfg.RouterID == "" {
		return fmt.Errorf("router-id is required for BGP instance")
	}

	return nil
}

func (h *BGPInstanceHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg := hctx.NewValue.(*config.BGPConfig)
	asn, err := extractASNFromPath(hctx.Path)
	if err != nil {
		return err
	}

	return h.routing.ConfigureBGP(asn, cfg.RouterID)
}

func (h *BGPInstanceHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	asn, err := extractASNFromPath(hctx.Path)
	if err != nil {
		return err
	}

	return h.routing.RemoveBGP(asn)
}

func extractASNFromPath(path string) (uint32, error) {
	var asn uint32
	_, err := fmt.Sscanf(path, "protocols.bgp.%d", &asn)
	if err != nil {
		return 0, fmt.Errorf("failed to extract ASN from path %s: %w", path, err)
	}
	return asn, nil
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
