package internal

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewBNGHandler)
}

type BNGHandler struct {
	southbound *southbound.VPP
}

func NewBNGHandler(d *deps.ConfDeps) conf.Handler {
	return &BNGHandler{
		southbound: d.Southbound,
	}
}

func (h *BNGHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*interfaces.BNGConfig)
	if !ok {
		return fmt.Errorf("expected *interfaces.BNGConfig, got %T", hctx.NewValue)
	}

	switch cfg.Mode {
	case interfaces.BNGModeIPoE, interfaces.BNGModeIPoEL3,
		interfaces.BNGModePPPoE, interfaces.BNGModeLAC, interfaces.BNGModeLNS:
		return nil
	default:
		return fmt.Errorf("invalid BNG mode: %s", cfg.Mode)
	}
}

func (h *BNGHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg := hctx.NewValue.(*interfaces.BNGConfig)

	values, err := paths.InterfaceSubinterfaceBNG.ExtractWildcards(hctx.Path, 2)
	if err != nil {
		return fmt.Errorf("extract interface from path: %w", err)
	}

	parentIface := values[0]
	vlanID := values[1]
	subIfName := fmt.Sprintf("%s.%s", parentIface, vlanID)

	switch cfg.Mode {
	case interfaces.BNGModeIPoE:
		if err := h.southbound.EnableDHCPv4Punt(subIfName); err != nil {
			return fmt.Errorf("enable dhcpv4 punt: %w", err)
		}
		if err := h.southbound.EnableDHCPv6Punt(subIfName); err != nil {
			return fmt.Errorf("enable dhcpv6 punt: %w", err)
		}
		if err := h.southbound.EnableARPPunt(subIfName); err != nil {
			return fmt.Errorf("enable arp punt: %w", err)
		}
		if err := h.southbound.EnableIPv6NDPunt(subIfName); err != nil {
			return fmt.Errorf("enable ipv6 nd punt: %w", err)
		}

	case interfaces.BNGModeIPoEL3:
		if err := h.southbound.EnableDHCPv4Punt(subIfName); err != nil {
			return fmt.Errorf("enable dhcpv4 punt: %w", err)
		}
		if err := h.southbound.EnableDHCPv6Punt(subIfName); err != nil {
			return fmt.Errorf("enable dhcpv6 punt: %w", err)
		}

	case interfaces.BNGModePPPoE:
		if err := h.southbound.SetInterfacePromiscuous(parentIface, true); err != nil {
			return fmt.Errorf("set promiscuous on %s: %w", parentIface, err)
		}
		if err := h.southbound.EnablePPPoEPunt(subIfName); err != nil {
			return fmt.Errorf("enable pppoe punt: %w", err)
		}

	case interfaces.BNGModeLAC:
		if err := h.southbound.SetInterfacePromiscuous(parentIface, true); err != nil {
			return fmt.Errorf("set promiscuous on %s: %w", parentIface, err)
		}
		if err := h.southbound.EnablePPPoEPunt(subIfName); err != nil {
			return fmt.Errorf("enable pppoe punt: %w", err)
		}

	case interfaces.BNGModeLNS:
		if err := h.southbound.EnableL2TPPunt(subIfName); err != nil {
			return fmt.Errorf("enable l2tp punt: %w", err)
		}
	}

	return nil
}

func (h *BNGHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *BNGHandler) PathPattern() paths.Path {
	return paths.InterfaceSubinterfaceBNG
}

func (h *BNGHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.InterfaceSubinterface}
}

func (h *BNGHandler) Callbacks() *conf.Callbacks {
	return nil
}
