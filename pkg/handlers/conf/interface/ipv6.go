package iface

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewInterfaceIPv6Handler)
	conf.RegisterFactory(NewSubinterfaceIPv6Handler)
}

type IPv6Handler struct {
	southbound     *southbound.VPP
	dataplaneState operations.DataplaneStateReader
	pathPattern    paths.Path
	dependencies   []paths.Path
}

func NewInterfaceIPv6Handler(d *deps.ConfDeps) conf.Handler {
	return &IPv6Handler{
		southbound:     d.Southbound,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InterfaceIPv6,
		dependencies:   []paths.Path{paths.Interface},
	}
}

func NewSubinterfaceIPv6Handler(d *deps.ConfDeps) conf.Handler {
	return &IPv6Handler{
		southbound:     d.Southbound,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InterfaceSubinterfaceIPv6,
		dependencies:   []paths.Path{paths.InterfaceSubinterface},
	}
}

func (h *IPv6Handler) extractInterfaceName(path string) (string, error) {
	values, err := h.pathPattern.ExtractWildcards(path, 2)
	if err != nil {
		return "", err
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return fmt.Sprintf("%s.%s", values[0], values[1]), nil
}

func (h *IPv6Handler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(*interfaces.IPv6Config)
	if !ok {
		return fmt.Errorf("expected *interfaces.IPv6Config, got %T", hctx.NewValue)
	}
	return nil
}

func (h *IPv6Handler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg := hctx.NewValue.(*interfaces.IPv6Config)

	ifName, err := h.extractInterfaceName(hctx.Path)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}

	if cfg.Enabled {
		needsEnable := true
		if h.dataplaneState != nil {
			ifState := h.dataplaneState.GetInterfaceByName(ifName)
			if ifState != nil && h.dataplaneState.IsIPv6Enabled(ifState.SwIfIndex) {
				needsEnable = false
			}
		}
		if needsEnable {
			if err := h.southbound.EnableIPv6(ifName); err != nil {
				return fmt.Errorf("enable ipv6: %w", err)
			}
		}
	}

	if cfg.RA != nil {
		raConfig := southbound.IPv6RAConfig{
			Managed:        cfg.RA.Managed,
			Other:          cfg.RA.Other,
			RouterLifetime: cfg.RA.RouterLifetime,
			MaxInterval:    cfg.RA.MaxInterval,
			MinInterval:    cfg.RA.MinInterval,
		}
		if err := h.southbound.ConfigureIPv6RA(ifName, raConfig); err != nil {
			return fmt.Errorf("configure ra: %w", err)
		}
	}

	if cfg.Multicast {
		needsMulticast := true
		if h.dataplaneState != nil {
			ifState := h.dataplaneState.GetInterfaceByName(ifName)
			if ifState != nil && h.dataplaneState.IsDHCPv6MulticastEnabled(ifState.SwIfIndex) {
				needsMulticast = false
			}
		}
		if needsMulticast {
			if err := h.southbound.EnableDHCPv6Multicast(ifName); err != nil {
				return fmt.Errorf("enable multicast: %w", err)
			}
		}
	}

	return nil
}

func (h *IPv6Handler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *IPv6Handler) PathPattern() paths.Path {
	return h.pathPattern
}

func (h *IPv6Handler) Dependencies() []paths.Path {
	return h.dependencies
}

func (h *IPv6Handler) Callbacks() *conf.Callbacks {
	return nil
}
