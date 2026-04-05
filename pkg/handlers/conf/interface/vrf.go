// Copyright 2026 Veesix Networks Ltd
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
	return h.southbound.BindInterfaceToVRF(ifName, vrfName, hasLCP)
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
