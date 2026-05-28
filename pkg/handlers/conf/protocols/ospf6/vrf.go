// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

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
	conf.RegisterFactory(NewOSPF6VRFHandler)
}

type OSPF6VRFHandler struct {
	southbound southbound.Southbound
	vrfMgr     *vrfmgr.Manager
}

func NewOSPF6VRFHandler(d *deps.ConfDeps) conf.Handler {
	return &OSPF6VRFHandler{
		southbound: d.Southbound,
		vrfMgr:     d.VRFManager,
	}
}

func (h *OSPF6VRFHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	cfg, ok := hctx.NewValue.(*protocols.OSPF6VRFConfig)
	if !ok {
		return fmt.Errorf("expected *protocols.OSPF6VRFConfig, got %T", hctx.NewValue)
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
			if ifc.Authentication != nil && !ifc.Authentication.HashAlgo.Valid() {
				return fmt.Errorf("area %s interface %s: invalid hash-algo %q", areaID, iface, ifc.Authentication.HashAlgo)
			}
		}
	}

	return nil
}

func (h *OSPF6VRFHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	vrfName, err := h.extractVRFName(hctx.Path)
	if err != nil {
		return err
	}

	tableID, _, _, err := h.vrfMgr.ResolveVRF(vrfName)
	if err != nil {
		return fmt.Errorf("resolve VRF %q for OSPFv3: %w", vrfName, err)
	}

	for _, group := range ospf6MulticastGroups {
		if err := h.southbound.AddMfibLocalReceiveAllInterfaces(group, tableID); err != nil {
			return fmt.Errorf("add OSPFv3 mfib local receive for %s in VRF %s: %w", group, vrfName, err)
		}
	}

	return nil
}

func (h *OSPF6VRFHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	return nil
}

func (h *OSPF6VRFHandler) PathPattern() paths.Path {
	return paths.OSPF6VRF
}

func (h *OSPF6VRFHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.VRFS}
}

func (h *OSPF6VRFHandler) Callbacks() *conf.Callbacks {
	return &conf.Callbacks{
		OnAfterApply: func(hctx *conf.HandlerContext, err error) {
			if err == nil {
				hctx.MarkFRRReloadNeeded()
			}
		},
	}
}

func (h *OSPF6VRFHandler) Summary() string {
	return "OSPFv3 instance bound to a VRF"
}

func (h *OSPF6VRFHandler) Description() string {
	return "Configure an OSPFv3 instance scoped to the named VRF (router ospf6 vrf <name>)."
}

func (h *OSPF6VRFHandler) ValueType() interface{} {
	return &protocols.OSPF6VRFConfig{}
}

func (h *OSPF6VRFHandler) extractVRFName(path string) (string, error) {
	values, err := paths.OSPF6VRF.ExtractWildcards(path, 1)
	if err != nil {
		return "", fmt.Errorf("extract VRF name from path %q: %w", path, err)
	}
	return values[0], nil
}
