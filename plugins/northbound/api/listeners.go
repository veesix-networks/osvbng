// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package api

import (
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/logger"
)

const legacyDeprecationMsg = "northbound.api: top-level listen_address/vrf/tls are deprecated; use listeners: []"

func (c *Config) hasLegacyListenerFields() bool {
	return c.ListenAddress != "" || c.ListenerBinding.VRF != "" || c.TLS.IsEnabled() ||
		c.TLS.CertFile != "" || c.TLS.KeyFile != "" || c.TLS.CACertFile != "" ||
		c.TLS.ClientAuth != "" || c.TLS.MinVersion != ""
}

func (c *Config) buildListeners() ([]ListenerConfig, error) {
	if len(c.Listeners) > 0 {
		if c.hasLegacyListenerFields() {
			return nil, fmt.Errorf("northbound.api: listeners is set; remove the deprecated top-level listen_address/vrf/tls fields")
		}
		return c.Listeners, nil
	}
	if !c.hasLegacyListenerFields() {
		return nil, nil
	}
	addr := c.ListenAddress
	if addr == "" {
		addr = ":8080"
	}
	return []ListenerConfig{{
		Address:         addr,
		ListenerBinding: c.ListenerBinding,
		TLS:             c.TLS,
	}}, nil
}

func (c *Config) resolveListeners(log *logger.Logger) ([]ListenerConfig, error) {
	out, err := c.buildListeners()
	if err != nil {
		return nil, err
	}
	if len(c.Listeners) == 0 && len(out) > 0 {
		log.Warn(legacyDeprecationMsg)
	}
	return out, nil
}

func (c *Config) validateListeners() error {
	listeners, err := c.buildListeners()
	if err != nil {
		return err
	}

	seen := make(map[string]int, len(listeners))
	for i, l := range listeners {
		if l.Address == "" {
			return fmt.Errorf("listeners[%d]: address is required", i)
		}
		if _, _, err := net.SplitHostPort(l.Address); err != nil {
			return fmt.Errorf("listeners[%d] address %q: %w", i, l.Address, err)
		}
		if prev, dup := seen[l.Address]; dup {
			return fmt.Errorf("listeners[%d] address %q duplicates listeners[%d]", i, l.Address, prev)
		}
		seen[l.Address] = i

		if err := l.TLS.Validate(); err != nil {
			return fmt.Errorf("listeners[%d] tls: %w", i, err)
		}
	}
	return nil
}
