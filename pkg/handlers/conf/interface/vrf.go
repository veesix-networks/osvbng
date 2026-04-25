// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package iface

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	conf.RegisterFactory(NewSubinterfaceVRFHandler)
}

type VRFHandler struct {
	southbound   southbound.Southbound
	pathPattern  paths.Path
	dependencies []paths.Path
}

func NewSubinterfaceVRFHandler(d *deps.ConfDeps) conf.Handler {
	return &VRFHandler{
		southbound:   d.Southbound,
		pathPattern:  paths.InterfaceSubinterfaceVRF,
		dependencies: []paths.Path{paths.InterfaceSubinterface},
	}
}

func (h *VRFHandler) extractInterfaceName(path string) (string, error) {
	values, err := h.pathPattern.ExtractWildcards(path, 2)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.%s", values[0], values[1]), nil
}

func (h *VRFHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	vrfName, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("vrf must be a string")
	}

	if hctx.Config != nil && hctx.Config.VRFS != nil {
		if _, exists := hctx.Config.VRFS[vrfName]; !exists {
			return fmt.Errorf("vrf %q does not exist", vrfName)
		}
	}

	if hctx.OldValue != nil {
		ifName, err := h.extractInterfaceName(hctx.Path)
		if err == nil {
			ifMgr := h.southbound.GetIfMgr()
			if iface := ifMgr.GetByName(ifName); iface != nil {
				if len(iface.IPv4Addresses) > 0 || len(iface.IPv6Addresses) > 0 {
					return fmt.Errorf("cannot change VRF on %s: remove all IP addresses first", ifName)
				}
			}
		}
	}

	return nil
}

func (h *VRFHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	vrfName := hctx.NewValue.(string)
	if vrfName == "" {
		return nil
	}

	ifName, err := h.extractInterfaceName(hctx.Path)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}

	hasLCP := h.southbound.HasLCPPair(ifName)
	if err := h.southbound.BindInterfaceToVRF(ifName, vrfName, hasLCP); err != nil {
		return err
	}

	return h.reconcileMulticast(ifName, hctx)
}

func (h *VRFHandler) reconcileMulticast(ifName string, hctx *conf.HandlerContext) error {
	if hctx.Config == nil {
		return nil
	}
	values, err := h.pathPattern.ExtractWildcards(hctx.Path, 2)
	if err != nil || len(values) != 2 {
		return nil
	}
	parent, ok := hctx.Config.Interfaces[values[0]]
	if !ok || parent == nil || parent.Subinterfaces == nil {
		return nil
	}
	sub, ok := parent.Subinterfaces[values[1]]
	if !ok || sub == nil || sub.IPv6 == nil || !sub.IPv6.Multicast {
		return nil
	}

	ifMgr := h.southbound.GetIfMgr()
	iface := ifMgr.GetByName(ifName)
	if iface == nil {
		return nil
	}
	tableID, err := h.southbound.GetFIBIDForInterface(iface.SwIfIndex)
	if err != nil {
		return nil
	}
	if err := h.southbound.DisableIPv6(ifName); err != nil {
		return fmt.Errorf("disable ipv6 for vrf rebind: %w", err)
	}
	if err := h.southbound.EnableIPv6(ifName); err != nil {
		return fmt.Errorf("re-enable ipv6 after vrf rebind: %w", err)
	}
	return h.southbound.EnableDHCPv6Multicast(ifName, tableID)
}

func (h *VRFHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *VRFHandler) PathPattern() paths.Path {
	return h.pathPattern
}

func (h *VRFHandler) Dependencies() []paths.Path {
	return h.dependencies
}

func (h *VRFHandler) Callbacks() *conf.Callbacks {
	return nil
}

func (h *VRFHandler) Summary() string {
	return "Interface VRF assignment"
}

func (h *VRFHandler) Description() string {
	return "Assign an interface to a VRF."
}

func (h *VRFHandler) ValueType() interface{} {
	return ""
}
