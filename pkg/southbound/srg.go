// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package southbound

import "net"

type SRGGarpEntry struct {
	SwIfIndex uint32
	IP        net.IP
}

type SRGCounters struct {
	SRGName    string
	GarpSent   uint64
	NaSent     uint64
	MacAdds    uint64
	MacRemoves uint64
}

type SRGDataplane interface {
	AddSRG(srgName string, virtualMAC net.HardwareAddr, swIfIndices []uint32) error
	DelSRG(srgName string) error
	SetSRGState(srgName string, isActive bool) error
	SendSRGGarp(srgName string, entries []SRGGarpEntry) error
	GetSRGCounters(srgName string) ([]SRGCounters, error)
}
