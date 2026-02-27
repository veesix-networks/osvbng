// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"net"

	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type NoopSRGDataplane struct{}

func (n *NoopSRGDataplane) AddSRG(string, net.HardwareAddr, []uint32) error        { return nil }
func (n *NoopSRGDataplane) DelSRG(string) error                                    { return nil }
func (n *NoopSRGDataplane) SetSRGState(string, bool) error                         { return nil }
func (n *NoopSRGDataplane) SendSRGGarp(string, []southbound.SRGGarpEntry) error    { return nil }
func (n *NoopSRGDataplane) GetSRGCounters(string) ([]southbound.SRGCounters, error) { return nil, nil }
