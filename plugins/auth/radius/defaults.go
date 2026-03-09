// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package radius

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/aaa"
	"layeh.com/radius"
)

type responseMapping struct {
	attrType byte
	internal string
	decode   func(radius.Attribute) string
}

type vendorMapping struct {
	vendorID   uint32
	vendorType byte
	internal   string
	decode     func([]byte) string
}

func decodeIPv4(a radius.Attribute) string {
	if len(a) != 4 {
		return ""
	}
	return net.IP(a).String()
}

func decodeString(a radius.Attribute) string {
	return string(a)
}

func decodeUint32(a radius.Attribute) string {
	if len(a) != 4 {
		return ""
	}
	return fmt.Sprintf("%d", binary.BigEndian.Uint32(a))
}

func decodeIPv6Address(a radius.Attribute) string {
	if len(a) != 16 {
		return ""
	}
	return net.IP(a).String()
}

// 1 byte reserved + 1 byte prefix-length + up to 16 bytes prefix
func decodeIPv6Prefix(a radius.Attribute) string {
	if len(a) < 4 {
		return ""
	}
	prefixLen := int(a[1])
	ipBytes := make([]byte, 16)
	copy(ipBytes, a[2:])
	return fmt.Sprintf("%s/%d", net.IP(ipBytes), prefixLen)
}

func decodeVSAIPv4(data []byte) string {
	if len(data) != 4 {
		return ""
	}
	return net.IP(data).String()
}

var tier1Mappings = []responseMapping{
	{attrType: 8, internal: aaa.AttrIPv4Address, decode: decodeIPv4},
	{attrType: 9, internal: aaa.AttrIPv4Netmask, decode: decodeIPv4},
	{attrType: 22, internal: aaa.AttrRoutedPrefix, decode: decodeString},
	{attrType: 27, internal: aaa.AttrSessionTimeout, decode: decodeUint32},
	{attrType: 28, internal: aaa.AttrIdleTimeout, decode: decodeUint32},
	{attrType: 85, internal: aaa.AttrAcctInterimInterval, decode: decodeUint32},
	{attrType: 88, internal: aaa.AttrPool, decode: decodeString},
	{attrType: 97, internal: aaa.AttrIPv6WANPrefix, decode: decodeIPv6Prefix},
	{attrType: 100, internal: aaa.AttrIANAPool, decode: decodeString},
	{attrType: 123, internal: aaa.AttrIPv6Prefix, decode: decodeIPv6Prefix},
	{attrType: 168, internal: aaa.AttrIPv6Address, decode: decodeIPv6Address},
	{attrType: 171, internal: aaa.AttrPDPool, decode: decodeString},
}

var tier2Mappings = []vendorMapping{
	{vendorID: 311, vendorType: 28, internal: aaa.AttrDNSPrimary, decode: decodeVSAIPv4},
	{vendorID: 311, vendorType: 29, internal: aaa.AttrDNSSecondary, decode: decodeVSAIPv4},
}

func buildTier1Index() map[byte]*responseMapping {
	idx := make(map[byte]*responseMapping, len(tier1Mappings))
	for i := range tier1Mappings {
		idx[tier1Mappings[i].attrType] = &tier1Mappings[i]
	}
	return idx
}

type vendorKey struct {
	vendorID   uint32
	vendorType byte
}

func buildTier2Index() map[vendorKey]*vendorMapping {
	idx := make(map[vendorKey]*vendorMapping, len(tier2Mappings))
	for i := range tier2Mappings {
		k := vendorKey{vendorID: tier2Mappings[i].vendorID, vendorType: tier2Mappings[i].vendorType}
		idx[k] = &tier2Mappings[i]
	}
	return idx
}
