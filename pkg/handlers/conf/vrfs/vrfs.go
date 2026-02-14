package vrfs

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/ip"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/models/vrf"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

func init() {
	conf.RegisterFactory(NewVRFHandler)
}

type VRFHandler struct {
	vrfMgr *vrfmgr.Manager
}

func NewVRFHandler(daemons *deps.ConfDeps) conf.Handler {
	return &VRFHandler{vrfMgr: daemons.VRFManager}
}

func (h *VRFHandler) extractVRFName(path string) (string, error) {
	values, err := paths.VRFS.ExtractWildcards(path, 1)
	if err != nil {
		return "", fmt.Errorf("extract VRF name from path: %w", err)
	}
	return values[0], nil
}

func (h *VRFHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	vrfName, err := h.extractVRFName(hctx.Path)
	if err != nil {
		return err
	}

	if err := vrf.ValidateVRFName(vrfName); err != nil {
		return err
	}

	if hctx.NewValue == nil {
		return nil
	}

	cfg, ok := hctx.NewValue.(*ip.VRFSConfig)
	if !ok {
		return fmt.Errorf("expected *ip.VRFSConfig, got %T", hctx.NewValue)
	}

	if cfg.AddressFamilies.IPv4Unicast == nil && cfg.AddressFamilies.IPv6Unicast == nil {
		return fmt.Errorf("VRF %q must have at least one address family configured", vrfName)
	}

	return nil
}

func (h *VRFHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	vrfName, err := h.extractVRFName(hctx.Path)
	if err != nil {
		return err
	}

	if hctx.NewValue == nil {
		return h.vrfMgr.DeleteVRF(ctx, vrfName)
	}

	cfg, ok := hctx.NewValue.(*ip.VRFSConfig)
	if !ok {
		return fmt.Errorf("expected *ip.VRFSConfig, got %T", hctx.NewValue)
	}

	ipv4 := cfg.AddressFamilies.IPv4Unicast != nil
	ipv6 := cfg.AddressFamilies.IPv6Unicast != nil

	_, err = h.vrfMgr.CreateVRF(ctx, vrfName, ipv4, ipv6)
	return err
}

func (h *VRFHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	vrfName, err := h.extractVRFName(hctx.Path)
	if err != nil {
		return err
	}

	if hctx.OldValue == nil {
		return h.vrfMgr.DeleteVRF(ctx, vrfName)
	}

	cfg, ok := hctx.OldValue.(*ip.VRFSConfig)
	if !ok {
		return nil
	}

	ipv4 := cfg.AddressFamilies.IPv4Unicast != nil
	ipv6 := cfg.AddressFamilies.IPv6Unicast != nil

	_, err = h.vrfMgr.CreateVRF(ctx, vrfName, ipv4, ipv6)
	return err
}

func (h *VRFHandler) PathPattern() paths.Path {
	return paths.VRFS
}

func (h *VRFHandler) Dependencies() []paths.Path {
	return nil
}

func (h *VRFHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}
