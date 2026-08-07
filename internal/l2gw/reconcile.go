// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2gw

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config"
	l2gwcfg "github.com/veesix-networks/osvbng/pkg/config/l2gw"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

// ReconcileConfig converges the component onto a committed l2gw config:
// static circuits are diffed against the desired maps (stale or drifted
// entries removed, missing ones installed) and the per-group egress
// allocators are rebuilt from the new ranges with live circuits' pairs
// re-marked. Installed dynamic circuits are deliberately left alone,
// they are session state, torn down via RADIUS Disconnect or operator
// action, not config edits.
func (c *Component) ReconcileConfig(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}

	c.rebuildAllocators(cfg.L2GW)

	if err := c.reconcileStaticMaps(cfg.L2GW); err != nil {
		return err
	}

	if cfg.SubscriberGroups != nil {
		seen := make(map[string]bool)
		for _, group := range cfg.SubscriberGroups.Groups {
			if group == nil {
				continue
			}
			for _, vr := range group.VLANs {
				if !vr.HasAccessType(subscriber.AccessTypeL2GW) || seen[vr.ParentInterface] {
					continue
				}
				seen[vr.ParentInterface] = true
				if err := c.armInterfaceTriggers(vr.ParentInterface); err != nil {
					c.logger.Error("Failed to arm l2gw triggers during reconcile", "interface", vr.ParentInterface, "error", err)
				}
			}
		}
	}

	c.logger.Info("Reconciled l2gw configuration")
	return nil
}

func (c *Component) rebuildAllocators(l2gwCfg *l2gwcfg.L2GWConfig) {
	c.allocMu.Lock()
	defer c.allocMu.Unlock()

	fresh := make(map[string]*vlanAllocator)
	if l2gwCfg != nil {
		for name := range c.allocators {
			hg, ok := l2gwCfg.HandoffGroups[name]
			if !ok {
				continue
			}
			a, err := newVlanAllocator(hg)
			if err != nil {
				c.logger.Error("Failed to rebuild l2gw allocator",
					"handoff_group", name, "error", err)
				continue
			}
			fresh[name] = a
		}
	}

	c.circuits.Range(func(_, v any) bool {
		ct := v.(*Circuit)
		ct.mu.Lock()
		group, svlan, cvlan, static := ct.HandoffGroup, ct.HandoffSVLAN, ct.HandoffCVLAN, ct.Static
		ct.mu.Unlock()
		if static {
			return true
		}
		if a, ok := fresh[group]; ok {
			a.mark(svlan, cvlan)
		}
		return true
	})

	c.allocators = fresh
}

type staticDesired struct {
	sm           *l2gwcfg.StaticMap
	hg           *l2gwcfg.HandoffGroup
	handoffIdx   uint32
	handoffSVLAN uint16
	transparent  bool
}

func (c *Component) reconcileStaticMaps(l2gwCfg *l2gwcfg.L2GWConfig) error {
	desired := make(map[string]staticDesired)
	if l2gwCfg != nil {
		for i, sm := range l2gwCfg.StaticMaps {
			if sm == nil {
				continue
			}
			d, keys, err := c.expandStaticMap(l2gwCfg, sm)
			if err != nil {
				return fmt.Errorf("static-map %d: %w", i, err)
			}
			for j, key := range keys {
				entry := d[j]
				desired[key] = entry
			}
		}
	}

	var stale []*Circuit
	c.circuits.Range(func(k, v any) bool {
		ct := v.(*Circuit)
		if !ct.Static {
			return true
		}
		want, ok := desired[k.(string)]
		if !ok || c.staticDrifted(ct, want) {
			stale = append(stale, ct)
		}
		return true
	})

	for _, ct := range stale {
		c.logger.Info("Removing stale static l2gw circuit",
			"access_interface", ct.AccessInterface, "svlan", ct.AccessSVLAN,
			"handoff_group", ct.HandoffGroup)
		c.removeCircuit(ct)
		c.circuits.Delete(ct.key())
	}

	return c.applyStaticMapsFrom(l2gwCfg)
}

func (c *Component) expandStaticMap(l2gwCfg *l2gwcfg.L2GWConfig, sm *l2gwcfg.StaticMap) ([]staticDesired, []string, error) {
	hg, ok := l2gwCfg.HandoffGroups[sm.HandoffGroup]
	if !ok {
		return nil, nil, fmt.Errorf("unknown handoff-group %q", sm.HandoffGroup)
	}
	accessIdx, ok := c.ifMgr.GetSwIfIndex(sm.AccessInterface)
	if !ok {
		return nil, nil, fmt.Errorf("access interface %q not found", sm.AccessInterface)
	}
	handoffIdx, ok := c.ifMgr.GetSwIfIndex(hg.Interface)
	if !ok {
		return nil, nil, fmt.Errorf("handoff interface %q not found", hg.Interface)
	}
	svlans, err := sm.GetSVLANs()
	if err != nil {
		return nil, nil, fmt.Errorf("svlan: %w", err)
	}

	transparent := sm.Transparent
	handoffSVLAN := sm.HandoffSVLAN
	if !transparent && handoffSVLAN == 0 {
		handoffSVLAN = hg.SVLAN
	}
	if !transparent && handoffSVLAN == 0 {
		transparent = true
	}

	out := make([]staticDesired, 0, len(svlans))
	keys := make([]string, 0, len(svlans))
	for _, svlan := range svlans {
		hs := handoffSVLAN
		if transparent {
			hs = svlan
		}
		out = append(out, staticDesired{
			sm:           sm,
			hg:           hg,
			handoffIdx:   handoffIdx,
			handoffSVLAN: hs,
			transparent:  transparent,
		})
		keys = append(keys, circuitKey(accessIdx, svlan, southbound.L2GWCvlanAny))
	}
	return out, keys, nil
}

func (c *Component) staticDrifted(ct *Circuit, want staticDesired) bool {
	return ct.HandoffIfIndex != want.handoffIdx ||
		ct.HandoffSVLAN != want.handoffSVLAN ||
		ct.Transparent != want.transparent ||
		ct.HandoffTPID != want.hg.GetOuterTPID() ||
		ct.HandoffGroup != want.sm.HandoffGroup
}
