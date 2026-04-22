// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package relay

import (
	"encoding/binary"
	"net"
)

const (
	OffsetHops   = 3
	OffsetGIAddr = 24
	OffsetOpts   = 240

	OptLeaseTime = 51
	OptServerID  = 54
	OptT1        = 58
	OptT2        = 59
)

func WrapIPUDP(dhcpPayload []byte, srcIP, dstIP net.IP) []byte {
	udpLen := 8 + len(dhcpPayload)
	ipLen := 20 + udpLen
	pkt := make([]byte, ipLen)

	src4 := srcIP.To4()
	dst4 := dstIP.To4()

	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:4], uint16(ipLen))
	pkt[8] = 64
	pkt[9] = 0x11
	copy(pkt[12:16], src4)
	copy(pkt[16:20], dst4)

	var csum uint32
	for i := 0; i < 20; i += 2 {
		csum += uint32(binary.BigEndian.Uint16(pkt[i : i+2]))
	}
	for csum > 0xffff {
		csum = (csum & 0xffff) + (csum >> 16)
	}
	binary.BigEndian.PutUint16(pkt[10:12], ^uint16(csum))

	binary.BigEndian.PutUint16(pkt[20:22], 67)
	binary.BigEndian.PutUint16(pkt[22:24], 68)
	binary.BigEndian.PutUint16(pkt[24:26], uint16(udpLen))

	copy(pkt[28:], dhcpPayload)

	binary.BigEndian.PutUint16(pkt[26:28], udpChecksum(src4, dst4, pkt[20:]))
	return pkt
}

func udpChecksum(srcIP, dstIP net.IP, udpPkt []byte) uint16 {
	var csum uint32
	csum += uint32(srcIP[0])<<8 | uint32(srcIP[1])
	csum += uint32(srcIP[2])<<8 | uint32(srcIP[3])
	csum += uint32(dstIP[0])<<8 | uint32(dstIP[1])
	csum += uint32(dstIP[2])<<8 | uint32(dstIP[3])
	csum += 0x11
	csum += uint32(len(udpPkt))

	for i := 0; i+1 < len(udpPkt); i += 2 {
		csum += uint32(binary.BigEndian.Uint16(udpPkt[i:]))
	}
	if len(udpPkt)%2 == 1 {
		csum += uint32(udpPkt[len(udpPkt)-1]) << 8
	}

	for csum > 0xffff {
		csum = (csum & 0xffff) + (csum >> 16)
	}
	res := ^uint16(csum)
	if res == 0 {
		return 0xffff
	}
	return res
}

func SetGIAddr(pkt []byte, giaddr net.IP) {
	if len(pkt) < 28 {
		return
	}
	copy(pkt[OffsetGIAddr:OffsetGIAddr+4], giaddr.To4())
}

func GetGIAddr(pkt []byte) net.IP {
	if len(pkt) < 28 {
		return nil
	}
	ip := make(net.IP, 4)
	copy(ip, pkt[OffsetGIAddr:OffsetGIAddr+4])
	return ip
}

func IncrementHops(pkt []byte) {
	if len(pkt) > OffsetHops {
		pkt[OffsetHops]++
	}
}

func GetHops(pkt []byte) uint8 {
	if len(pkt) > OffsetHops {
		return pkt[OffsetHops]
	}
	return 0
}

func SetOptionUint32(pkt []byte, optCode byte, value uint32) []byte {
	offset := findOption(pkt, optCode)
	if offset >= 0 && offset+5 < len(pkt) && pkt[offset+1] == 4 {
		binary.BigEndian.PutUint32(pkt[offset+2:offset+6], value)
		return pkt
	}
	return insertOption(pkt, optCode, 4, func(buf []byte) {
		binary.BigEndian.PutUint32(buf, value)
	})
}

func SetOptionIP(pkt []byte, optCode byte, addr net.IP) []byte {
	ip4 := addr.To4()
	if ip4 == nil {
		return pkt
	}
	offset := findOption(pkt, optCode)
	if offset >= 0 && offset+5 < len(pkt) && pkt[offset+1] == 4 {
		copy(pkt[offset+2:offset+6], ip4)
		return pkt
	}
	return insertOption(pkt, optCode, 4, func(buf []byte) {
		copy(buf, ip4)
	})
}

func GetOptionUint32(pkt []byte, optCode byte) (uint32, bool) {
	offset := findOption(pkt, optCode)
	if offset < 0 || offset+5 >= len(pkt) || pkt[offset+1] != 4 {
		return 0, false
	}
	return binary.BigEndian.Uint32(pkt[offset+2 : offset+6]), true
}

func GetOptionIP(pkt []byte, optCode byte) net.IP {
	offset := findOption(pkt, optCode)
	if offset < 0 || offset+5 >= len(pkt) || pkt[offset+1] != 4 {
		return nil
	}
	ip := make(net.IP, 4)
	copy(ip, pkt[offset+2:offset+6])
	return ip
}

func RewriteForProxy(pkt []byte, serverID net.IP, clientLease uint32) []byte {
	pkt = SetOptionIP(pkt, OptServerID, serverID)
	pkt = SetOptionUint32(pkt, OptLeaseTime, clientLease)
	pkt = SetOptionUint32(pkt, OptT1, clientLease/2)
	pkt = SetOptionUint32(pkt, OptT2, clientLease*7/8)
	return pkt
}

func findOption(pkt []byte, optCode byte) int {
	if len(pkt) < OffsetOpts {
		return -1
	}
	i := OffsetOpts
	for i < len(pkt) {
		if pkt[i] == 0 {
			i++
			continue
		}
		if pkt[i] == OptEnd {
			return -1
		}
		if i+1 >= len(pkt) {
			return -1
		}
		if pkt[i] == optCode {
			return i
		}
		i += 2 + int(pkt[i+1])
	}
	return -1
}

func insertOption(pkt []byte, optCode byte, dataLen int, fill func([]byte)) []byte {
	endIdx := len(pkt)
	if len(pkt) >= OffsetOpts {
		i := OffsetOpts
		for i < len(pkt) {
			if pkt[i] == 0 {
				i++
				continue
			}
			if pkt[i] == OptEnd {
				endIdx = i
				break
			}
			if i+1 >= len(pkt) {
				break
			}
			i += 2 + int(pkt[i+1])
		}
	}

	optTLV := make([]byte, 2+dataLen)
	optTLV[0] = optCode
	optTLV[1] = byte(dataLen)
	fill(optTLV[2:])

	result := make([]byte, 0, len(pkt)+len(optTLV))
	result = append(result, pkt[:endIdx]...)
	result = append(result, optTLV...)
	result = append(result, pkt[endIdx:]...)
	return result
}
