// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

import (
	"context"
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

func init() {
	conf.RegisterFactory(NewOSPFVRFHandler)
}

type OSPFVRFHandler struct {
	southbound southbound.Southbound
	vrfMgr     *vrfmgr.Manager
}

func NewOSPFVRFHandler(d *deps.ConfDeps) conf.Handler {
	return &OSPFVRFHandler{
		southbound: d.Southbound,
		vrfMgr:     d.VRFManager,
	}
}

func (h *OSPFVRFHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*protocols.OSPFVRFConfig)
	if !ok {
		return fmt.Errorf("expected *protocols.OSPFVRFConfig, got %T", hctx.NewValue)
	}
	if cfg == nil {
		return nil
	}

	if cfg.RouterID != "" && net.ParseIP(cfg.RouterID) == nil {
		return fmt.Errorf("invalid router-id %q: must be A.B.C.D format", cfg.RouterID)
	}

	for areaID, area := range cfg.Areas {
		if area == nil {
			continue
		}
		if area.Authentication != "" && !area.Authentication.Valid() {
			return fmt.Errorf("area %s: invalid authentication mode %q", areaID, area.Authentication)
		}
		for iface, ifc := range area.Interfaces {
			if ifc == nil {
				continue
			}
			if ifc.Network != "" && !ifc.Network.Valid() {
				return fmt.Errorf("area %s interface %s: invalid network type %q", areaID, iface, ifc.Network)
			}
			if ifc.HelloInterval != 0 && (ifc.HelloInterval > 65535) {
				return fmt.Errorf("area %s interface %s: hello-interval %d out of range (1-65535)", areaID, iface, ifc.HelloInterval)
			}
			if ifc.DeadInterval != 0 && (ifc.DeadInterval > 65535) {
				return fmt.Errorf("area %s interface %s: dead-interval %d out of range (1-65535)", areaID, iface, ifc.DeadInterval)
			}
			if ifc.Authentication != nil && !ifc.Authentication.Mode.Valid() {
				return fmt.Errorf("area %s interface %s: invalid auth mode %q", areaID, iface, ifc.Authentication.Mode)
			}
		}
	}

	return nil
}

func (h *OSPFVRFHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	vrfName, err := h.extractVRFName(hctx.Path)
	if err != nil {
		return err
	}

	tableID, _, _, err := h.vrfMgr.ResolveVRF(vrfName)
	if err != nil {
		return fmt.Errorf("resolve VRF %q for OSPF: %w", vrfName, err)
	}

	for _, group := range ospfMulticastGroups {
		if err := h.southbound.AddMfibLocalReceiveAllInterfaces(group, tableID); err != nil {
			return fmt.Errorf("add OSPF mfib local receive for %s in VRF %s: %w", group, vrfName, err)
		}
	}

	return nil
}

func (h *OSPFVRFHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPFVRFHandler) PathPattern() paths.Path {
	return paths.OSPFVRF
}

func (h *OSPFVRFHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.VRFS}
}

func (h *OSPFVRFHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}

func (h *OSPFVRFHandler) Summary() string {
	return "OSPFv2 instance bound to a VRF"
}

func (h *OSPFVRFHandler) Description() string {
	return "Configure an OSPFv2 instance scoped to the named VRF (router ospf vrf <name>)."
}

func (h *OSPFVRFHandler) ValueType() interface{} {
	return &protocols.OSPFVRFConfig{}
}

func (h *OSPFVRFHandler) extractVRFName(path string) (string, error) {
	values, err := paths.OSPFVRF.ExtractWildcards(path, 1)
	if err != nil {
		return "", fmt.Errorf("extract VRF name from path %q: %w", path, err)
	}
	return values[0], nil
}
