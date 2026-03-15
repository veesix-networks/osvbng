// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package models

import "net"

type CGNATMapping struct {
	PoolName       string `json:"pool_name"`
	PoolID         uint32 `json:"pool_id"`
	InsideIP       net.IP `json:"inside_ip"`
	InsideVRFID    uint32 `json:"inside_vrf_id"`
	OutsideIP      net.IP `json:"outside_ip"`
	PortBlockStart uint16 `json:"port_block_start"`
	PortBlockEnd   uint16 `json:"port_block_end"`
	SwIfIndex      uint32 `json:"sw_if_index"`
}

type CGNATPoolStats struct {
	Name               string  `json:"name"`
	Mode               string  `json:"mode"`
	TotalAddresses     uint32  `json:"total_addresses"`
	AllocatedAddresses uint32  `json:"allocated_addresses"`
	FreeBlocks         uint32  `json:"free_blocks"`
	TotalBlocks        uint32  `json:"total_blocks"`
	ExcludedAddresses  uint32  `json:"excluded_addresses"`
	SubscriberCount    uint32  `json:"subscriber_count"`
	Utilization        float64 `json:"utilization"`
}

type CGNATSessionInfo struct {
	OutsideIP net.IP `json:"outside_ip"`
	PortStart uint16 `json:"port_start"`
	PortEnd   uint16 `json:"port_end"`
	Pool      string `json:"pool"`
	Mode      string `json:"mode"`
}

type CGNATBypassEntry struct {
	Prefix      string `json:"prefix"`
	InsideVRFID uint32 `json:"inside_vrf_id"`
}
