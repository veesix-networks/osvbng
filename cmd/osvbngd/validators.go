// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/netbind"
)

// init registers post-vrfmgr validators for top-level config sections
// that don't live in a plugin namespace (HA, system.api). Plugin-owned
// validators register from their own package init.
func init() {
	configmgr.RegisterPostVRFValidator("ha", validateHABindings)
	configmgr.RegisterPostVRFValidator("system.api", validateAPIBinding)
}

func validateHABindings(cfg *config.Config, vrfMgr netbind.VRFResolver, nl netbind.LinkLister) error {
	if !cfg.HA.Enabled {
		return nil
	}
	if err := cfg.HA.Listen.Validate(addrFamily(cfg.HA.GetListenAddress()), vrfMgr, nl); err != nil {
		return fmt.Errorf("ha.listen: %w", err)
	}
	if cfg.HA.Peer.Address == "" {
		return nil
	}
	if err := cfg.HA.Peer.Validate(addrFamily(cfg.HA.Peer.Address), vrfMgr, nl); err != nil {
		return fmt.Errorf("ha.peer: %w", err)
	}
	return nil
}

func validateAPIBinding(cfg *config.Config, vrfMgr netbind.VRFResolver, nl netbind.LinkLister) error {
	if cfg.API.EndpointBinding.IsZero() {
		return nil
	}
	if err := cfg.API.EndpointBinding.Validate(addrFamily(cfg.API.Address), vrfMgr, nl); err != nil {
		return fmt.Errorf("api: %w", err)
	}
	return nil
}

func addrFamily(addr string) netbind.Family {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return netbind.FamilyV6
	}
	return netbind.FamilyV4
}
