// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package relay

import (
	"encoding/binary"
)

const (
	DHCPv6OptClientID = 1
	DHCPv6OptServerID = 2
	DHCPv6OptIANA     = 3
	DHCPv6OptIATA     = 4
	DHCPv6OptIAAddr   = 5
	DHCPv6OptIAPD     = 25
	DHCPv6OptIAPrefix = 26
)

// RewriteV6Lifetimes replaces all IA_NA/IA_PD T1/T2 and IAAddr/IAPrefix
// preferred/valid lifetimes in a DHCPv6 message with proxy-side values.
func RewriteV6Lifetimes(pkt []byte, preferredLifetime, validLifetime uint32) []byte {
	if len(pkt) < 4 {
		return pkt
	}
	rewriteV6Options(pkt[4:], preferredLifetime, validLifetime)
	return pkt
}

func rewriteV6Options(data []byte, pref, valid uint32) {
	i := 0
	for i+4 <= len(data) {
		optCode := binary.BigEndian.Uint16(data[i:])
		optLen := int(binary.BigEndian.Uint16(data[i+2:]))
		if i+4+optLen > len(data) {
			return
		}

		optData := data[i+4 : i+4+optLen]

		switch optCode {
		case DHCPv6OptIANA:
			// IA_NA: IAID(4) + T1(4) + T2(4) + IA options
			if len(optData) >= 12 {
				binary.BigEndian.PutUint32(optData[4:8], pref/2)
				binary.BigEndian.PutUint32(optData[8:12], pref*4/5)
				if len(optData) > 12 {
					rewriteV6Options(optData[12:], pref, valid)
				}
			}
		case DHCPv6OptIAPD:
			// IA_PD: IAID(4) + T1(4) + T2(4) + IA options
			if len(optData) >= 12 {
				binary.BigEndian.PutUint32(optData[4:8], pref/2)
				binary.BigEndian.PutUint32(optData[8:12], pref*4/5)
				if len(optData) > 12 {
					rewriteV6Options(optData[12:], pref, valid)
				}
			}
		case DHCPv6OptIAAddr:
			// IAAddr: address(16) + preferred(4) + valid(4) + options
			if len(optData) >= 24 {
				binary.BigEndian.PutUint32(optData[16:20], pref)
				binary.BigEndian.PutUint32(optData[20:24], valid)
			}
		case DHCPv6OptIAPrefix:
			// IAPrefix: preferred(4) + valid(4) + prefix-len(1) + prefix(16) + options
			if len(optData) >= 8 {
				binary.BigEndian.PutUint32(optData[0:4], pref)
				binary.BigEndian.PutUint32(optData[4:8], valid)
			}
		}

		i += 4 + optLen
	}
}

// ReplaceServerDUID replaces the Server Identifier option (2) in a DHCPv6
// message with the given DUID.
func ReplaceServerDUID(pkt []byte, newDUID []byte) []byte {
	if len(pkt) < 4 {
		return pkt
	}

	i := 4 // skip msg-type(1) + txn-id(3)
	for i+4 <= len(pkt) {
		optCode := binary.BigEndian.Uint16(pkt[i:])
		optLen := int(binary.BigEndian.Uint16(pkt[i+2:]))
		if i+4+optLen > len(pkt) {
			break
		}

		if optCode == DHCPv6OptServerID {
			if optLen == len(newDUID) {
				copy(pkt[i+4:i+4+optLen], newDUID)
				return pkt
			}

			// Different length: rebuild packet
			result := make([]byte, 0, len(pkt)-optLen+len(newDUID))
			result = append(result, pkt[:i+2]...)
			lenBuf := make([]byte, 2)
			binary.BigEndian.PutUint16(lenBuf, uint16(len(newDUID)))
			result = append(result, lenBuf...)
			result = append(result, newDUID...)
			result = append(result, pkt[i+4+optLen:]...)
			return result
		}

		i += 4 + optLen
	}
	return pkt
}

// GetServerDUID extracts the Server Identifier option (2) from a DHCPv6 message.
func GetServerDUID(pkt []byte) []byte {
	if len(pkt) < 4 {
		return nil
	}

	i := 4
	for i+4 <= len(pkt) {
		optCode := binary.BigEndian.Uint16(pkt[i:])
		optLen := int(binary.BigEndian.Uint16(pkt[i+2:]))
		if i+4+optLen > len(pkt) {
			return nil
		}
		if optCode == DHCPv6OptServerID {
			duid := make([]byte, optLen)
			copy(duid, pkt[i+4:i+4+optLen])
			return duid
		}
		i += 4 + optLen
	}
	return nil
}
