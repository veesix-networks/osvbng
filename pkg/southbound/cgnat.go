// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package southbound

import (
	"net"
)

type CGNATMapping struct {
	PoolID         uint32
	SwIfIndex      uint32
	InsideIP       net.IP
	InsideVRFID    uint32
	OutsideIP      net.IP
	PortBlockStart uint16
	PortBlockEnd   uint16
	EnableFeature  bool
	ActiveSessions uint32
}

type CGNATPoolState struct {
	PoolID             uint32
	Mode               uint8
	AddressPooling     uint8
	Filtering          uint8
	BlockSize          uint16
	MaxBlocksPerSub    uint8
	MaxSessionsPerSub  uint32
	PortRangeStart     uint16
	PortRangeEnd       uint16
	PortReuseTimeout   uint16
	ALGBitmask         uint8
	Timeouts           [4]uint32
	OutsideVRFTableID  uint32
	ActiveMappings     uint32
}

type CGNATInsidePrefixState struct {
	PoolID uint32
	Prefix net.IPNet
	VRFID  uint32
}

type CGNATOutsideAddressState struct {
	PoolID uint32
	Prefix net.IPNet
}

// CGNATSessionFilter narrows a session dump. Zero-valued fields are sentinels
// meaning "do not filter on this field" (0.0.0.0 for IPs, 0 for ports/proto/
// pool). StartIndex is the resume cursor (a session pool index); Limit caps the
// page (0 = plugin default). The plugin filters and windows the walk, so only
// matching, in-window sessions cross the API boundary.
type CGNATSessionFilter struct {
	InsideIP    net.IP
	OutsideIP   net.IP
	RemoteIP    net.IP
	InsidePort  uint16
	OutsidePort uint16
	RemotePort  uint16
	Proto       uint8
	PoolID      uint32
	StartIndex  uint32
	Limit       uint32
}

// CGNATSession is one active translation. Ports are host-order. Age is seconds
// since the session was last active (computed plugin-side at dump time).
type CGNATSession struct {
	SessionIndex   uint32
	PoolID         uint32
	InsideIP       net.IP
	OutsideIP      net.IP
	RemoteIP       net.IP
	InsidePort     uint16
	OutsidePort    uint16
	RemotePort     uint16
	Proto          uint8
	ALGFlags       uint8
	InsideFIBIndex uint32
	MappingIndex   uint32
	Age            float64
	Timeout        uint32
	TotalPackets   uint64
	TotalBytes     uint64
}

type CGNATDataplane interface {
	CGNATPoolAddDel(poolID uint32, mode uint8, addressPooling uint8,
		filtering uint8, blockSize uint16, maxBlocksPerSub uint8,
		maxSessionsPerSub uint32, portRangeStart uint16, portRangeEnd uint16,
		portReuseTimeout uint16, algBitmask uint8, timeouts [4]uint32,
		isAdd bool) error

	CGNATPoolAddInsidePrefix(poolID uint32, prefix net.IPNet, vrfID uint32, isAdd bool) error
	CGNATPoolAddOutsideAddress(poolID uint32, prefix net.IPNet, isAdd bool) error
	CGNATSetOutsideVRF(poolID uint32, vrfTableID uint32) error
	CGNATPoolUpdate(poolID uint32, maxSessions uint32, algBitmask uint8, timeouts [4]uint32) error

	CGNATAddDelSubscriberMapping(poolID uint32, swIfIndex uint32,
		insideIP net.IP, insideVRFID uint32, outsideIP net.IP,
		portStart uint16, portEnd uint16, enableFeature bool, isAdd bool) error

	CGNATAddDelSubscriberMappingAsync(poolID uint32, swIfIndex uint32,
		insideIP net.IP, insideVRFID uint32, outsideIP net.IP,
		portStart uint16, portEnd uint16, enableFeature bool, isAdd bool,
		callback func(error))

	// CGNATAddSubscriberMappingBulk programs N subscriber mappings under one
	// poolID. Returns a per-mapping error slice (len == len(mappings); nil entry
	// = success) and an overall error for unrecoverable framing failures only.
	// Per-mapping -116 (entry already exists) is surfaced as success.
	CGNATAddSubscriberMappingBulk(poolID uint32, mappings []CGNATMapping) ([]error, error)

	CGNATEnableOnSession(poolID uint32, swIfIndex uint32, isEnable bool) error

	CGNATAddDelBypass(prefix net.IPNet, vrfID uint32, isAdd bool) error

	CGNATDumpSubscriberMappings(poolID uint32) ([]CGNATMapping, error)
	CGNATPoolDump() ([]CGNATPoolState, error)
	CGNATPoolInsidePrefixDump(poolID uint32) ([]CGNATInsidePrefixState, error)
	CGNATPoolOutsideAddressDump(poolID uint32) ([]CGNATOutsideAddressState, error)

	CGNATDumpSessions(f CGNATSessionFilter) ([]CGNATSession, error)
	CGNATSessionCount() (uint64, error)
}
