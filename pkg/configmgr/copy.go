// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package configmgr

import (
	"encoding/json"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
)

// deepCopyConfig returns an in-memory clone of src. Uses JSON round-trip
// for the bulk of the structure but explicitly preserves the
// non-serialised flags (LCP, SubscriberAccess, MSSClamp) via a sidecar
// snapshot so callers that read these in-memory-only fields off the clone
// see the same values as the source.
//
// Why this matters:
//   - Autoconfig stamps SubscriberAccess=true on the subinterfaces it
//     derives from subscriber-group VLAN expansions.
//   - The sub-interface validator (pkg/handlers/conf/interface/subinterfaces)
//     rejects any non-SubscriberAccess subinterface whose VLAN overlaps a
//     subscriber-group expansion.
//   - A plain JSON round-trip here drops the `json:"-"` SubscriberAccess
//     flag, the cloned subinterface then looks like a regular
//     operator-authored entry, and the validator rejects it on reload as
//     a conflict — preserving the flag closes that gap.
//
// Save-to-disk has a separate concern (scrubPersistedConfig) that drops
// the derived subinterface entries entirely before writing to startup.yaml
// so they can be regenerated cleanly by autoconfig on next bootstrap.
func (cd *ConfigManager) deepCopyConfig(src *config.Config) *config.Config {
	if src == nil {
		return nil
	}

	hidden := captureHiddenInterfaceState(src)

	data, err := json.Marshal(src)
	if err != nil {
		panic(err)
	}

	dst := &config.Config{}
	if err := json.Unmarshal(data, dst); err != nil {
		panic(err)
	}

	restoreHiddenInterfaceState(dst, hidden)
	return dst
}

// hiddenInterfaceState snapshots the in-memory-only flags on every
// interface and subinterface of a Config. Used by deepCopyConfig to bridge
// fields that the JSON round-trip drops.
type hiddenInterfaceState struct {
	// iface[name] -> LCP
	iface map[string]bool
	// subif[parentName][subifKey] -> {LCP, SubscriberAccess, MSSClamp}
	subif map[string]map[string]hiddenSubif
}

type hiddenSubif struct {
	LCP              bool
	SubscriberAccess bool
	MSSClamp         *interfaces.MSSClampSpec
}

func captureHiddenInterfaceState(cfg *config.Config) *hiddenInterfaceState {
	h := &hiddenInterfaceState{
		iface: make(map[string]bool),
		subif: make(map[string]map[string]hiddenSubif),
	}
	for name, ic := range cfg.Interfaces {
		if ic == nil {
			continue
		}
		h.iface[name] = ic.LCP
		if len(ic.Subinterfaces) == 0 {
			continue
		}
		sub := make(map[string]hiddenSubif, len(ic.Subinterfaces))
		for key, sif := range ic.Subinterfaces {
			if sif == nil {
				continue
			}
			sub[key] = hiddenSubif{
				LCP:              sif.LCP,
				SubscriberAccess: sif.SubscriberAccess,
				MSSClamp:         sif.MSSClamp,
			}
		}
		h.subif[name] = sub
	}
	return h
}

func restoreHiddenInterfaceState(cfg *config.Config, h *hiddenInterfaceState) {
	for name, ic := range cfg.Interfaces {
		if ic == nil {
			continue
		}
		if v, ok := h.iface[name]; ok {
			ic.LCP = v
		}
		sub, ok := h.subif[name]
		if !ok {
			continue
		}
		for key, sif := range ic.Subinterfaces {
			if sif == nil {
				continue
			}
			if saved, ok := sub[key]; ok {
				sif.LCP = saved.LCP
				sif.SubscriberAccess = saved.SubscriberAccess
				sif.MSSClamp = saved.MSSClamp
			}
		}
	}
}

// scrubPersistedConfig returns a clone of cfg with autoconfig-derived
// subinterfaces stripped, suitable for writing to startup.yaml. On next
// bootstrap, autoconfig re-derives those entries from the subscriber-group
// configuration, avoiding the validator conflict that would otherwise
// fire when the persisted entries are loaded back without the
// SubscriberAccess flag (stripped by YAML marshal). Operator-authored
// subinterfaces (SubscriberAccess false or unset) are preserved
// verbatim.
func (cd *ConfigManager) scrubPersistedConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}
	out := cd.deepCopyConfig(cfg)
	for _, ic := range out.Interfaces {
		if ic == nil || len(ic.Subinterfaces) == 0 {
			continue
		}
		for key, sif := range ic.Subinterfaces {
			if sif != nil && sif.SubscriberAccess {
				delete(ic.Subinterfaces, key)
			}
		}
	}
	return out
}
