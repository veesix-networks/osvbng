// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package radius

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"

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

func encodeVSARequest(vendorID uint32, vendorType byte, data []byte) radius.Attribute {
	buf := make([]byte, 4+2+len(data))
	binary.BigEndian.PutUint32(buf[0:4], vendorID)
	buf[4] = vendorType
	buf[5] = byte(2 + len(data))
	copy(buf[6:], data)
	return radius.Attribute(buf)
}

var standardAttrNames = map[string]radius.Type{
	"User-Name":              1,
	"User-Password":          2,
	"CHAP-Password":          3,
	"NAS-IP-Address":         4,
	"NAS-Port":               5,
	"Service-Type":           6,
	"Framed-Protocol":        7,
	"Framed-IP-Address":      8,
	"Framed-IP-Netmask":      9,
	"Framed-Routing":         10,
	"Filter-Id":              11,
	"Framed-MTU":             12,
	"Reply-Message":          18,
	"State":                  24,
	"Class":                  25,
	"Session-Timeout":        27,
	"Idle-Timeout":           28,
	"Called-Station-Id":      30,
	"Calling-Station-Id":     31,
	"NAS-Identifier":         32,
	"Acct-Session-Id":        44,
	"Acct-Interim-Interval":  85,
	"NAS-Port-Id":            87,
	"NAS-Port-Type":          61,
	"Event-Timestamp":        55,
	"Connect-Info":           77,
	"Framed-IPv6-Prefix":     97,
	"Delegated-IPv6-Prefix":  123,
	"Framed-IPv6-Address":    168,
}

func resolveAttrName(name string) (radius.Type, bool) {
	if t, ok := standardAttrNames[name]; ok {
		return t, true
	}
	v, err := strconv.Atoi(name)
	if err != nil || v < 1 || v > 255 {
		return 0, false
	}
	return radius.Type(v), true
}

func encodeIPv6Prefix(cidr string) radius.Attribute {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil
	}
	ones, _ := ipNet.Mask.Size()
	buf := make([]byte, 2+16)
	buf[1] = byte(ones)
	copy(buf[2:], ipNet.IP.To16())
	return radius.Attribute(buf)
}
