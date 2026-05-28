// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/protocols"
)

func (c *Config) validateOSPFVRFInterfaces() error {
	if c.Protocols.OSPF != nil {
		if err := c.validateOSPFv2VRFInterfaces(); err != nil {
			return err
		}
	}
	if c.Protocols.OSPF6 != nil {
		if err := c.validateOSPFv3VRFInterfaces(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateOSPFv2VRFInterfaces() error {
	cfg := c.Protocols.OSPF
	globalIfaces := ospfGlobalInterfaces(cfg)

	for vrfName, vrfCfg := range cfg.VRF {
		if vrfCfg == nil {
			continue
		}
		for areaID, area := range vrfCfg.Areas {
			if area == nil {
				continue
			}
			for iface := range area.Interfaces {
				if _, ok := globalIfaces[iface]; ok {
					return fmt.Errorf("protocols.ospf: interface %q referenced under vrf.%s area %s but also under global areas",
						iface, vrfName, areaID)
				}
				declared, ok := c.lookupInterfaceVRF(iface)
				if !ok {
					return fmt.Errorf("protocols.ospf.vrf.%s.areas.%s.interfaces: interface %q is not declared in interfaces:",
						vrfName, areaID, iface)
				}
				if declared != vrfName {
					return fmt.Errorf("protocols.ospf.vrf.%s.areas.%s.interfaces: interface %q is declared with vrf=%q, must match",
						vrfName, areaID, iface, declared)
				}
			}
		}
	}
	return nil
}

func (c *Config) validateOSPFv3VRFInterfaces() error {
	cfg := c.Protocols.OSPF6
	globalIfaces := ospf6GlobalInterfaces(cfg)

	for vrfName, vrfCfg := range cfg.VRF {
		if vrfCfg == nil {
			continue
		}
		for areaID, area := range vrfCfg.Areas {
			if area == nil {
				continue
			}
			for iface := range area.Interfaces {
				if _, ok := globalIfaces[iface]; ok {
					return fmt.Errorf("protocols.ospf6: interface %q referenced under vrf.%s area %s but also under global areas",
						iface, vrfName, areaID)
				}
				declared, ok := c.lookupInterfaceVRF(iface)
				if !ok {
					return fmt.Errorf("protocols.ospf6.vrf.%s.areas.%s.interfaces: interface %q is not declared in interfaces:",
						vrfName, areaID, iface)
				}
				if declared != vrfName {
					return fmt.Errorf("protocols.ospf6.vrf.%s.areas.%s.interfaces: interface %q is declared with vrf=%q, must match",
						vrfName, areaID, iface, declared)
				}
			}
		}
	}
	return nil
}

func ospfGlobalInterfaces(cfg *protocols.OSPFConfig) map[string]struct{} {
	out := map[string]struct{}{}
	for _, area := range cfg.Areas {
		if area == nil {
			continue
		}
		for iface := range area.Interfaces {
			out[iface] = struct{}{}
		}
	}
	return out
}

func ospf6GlobalInterfaces(cfg *protocols.OSPF6Config) map[string]struct{} {
	out := map[string]struct{}{}
	for _, area := range cfg.Areas {
		if area == nil {
			continue
		}
		for iface := range area.Interfaces {
			out[iface] = struct{}{}
		}
	}
	return out
}

func (c *Config) lookupInterfaceVRF(name string) (string, bool) {
	for parentName, iface := range c.Interfaces {
		if iface == nil {
			continue
		}
		if parentName == name {
			return iface.VRF, true
		}
		for subID, sub := range iface.Subinterfaces {
			if sub == nil {
				continue
			}
			subName := fmt.Sprintf("%s.%s", parentName, subID)
			if subName == name {
				return sub.VRF, true
			}
		}
	}
	return "", false
}
