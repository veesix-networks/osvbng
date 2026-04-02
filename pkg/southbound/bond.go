// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package southbound

type BondInterfaceInfo struct {
	SwIfIndex     uint32
	Name          string
	Mode          string
	LoadBalance   string
	Members       uint32
	ActiveMembers uint32
}

type BondMemberInfo struct {
	SwIfIndex     uint32
	Name          string
	IsPassive     bool
	IsLongTimeout bool
	IsLocalNuma   bool
	Weight        uint32
}

type LACPInterfaceInfo struct {
	SwIfIndex             uint32
	Name                  string
	BondName              string
	RxState               uint32
	TxState               uint32
	MuxState              uint32
	PtxState              uint32
	ActorSystemPriority   uint16
	ActorSystem           string
	ActorKey              uint16
	ActorPortPriority     uint16
	ActorPortNumber       uint16
	ActorState            uint8
	PartnerSystemPriority uint16
	PartnerSystem         string
	PartnerKey            uint16
	PartnerPortPriority   uint16
	PartnerPortNumber     uint16
	PartnerState          uint8
}
