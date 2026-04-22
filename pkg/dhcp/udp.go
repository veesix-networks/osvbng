// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dhcp

import (
	"encoding/binary"
	"net"
)

func BuildIPv4UDPFrame(srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte) []byte {
	src4 := srcIP.To4()
	dst4 := dstIP.To4()
	if src4 == nil || dst4 == nil {
		return nil
	}

	udpLen := 8 + len(payload)
	totalLen := 20 + udpLen

	buf := make([]byte, totalLen)

	buf[0] = 0x45
	binary.BigEndian.PutUint16(buf[2:4], uint16(totalLen))
	buf[8] = 64
	buf[9] = 17
	copy(buf[12:16], src4)
	copy(buf[16:20], dst4)

	headerChecksum := computeIPv4HeaderChecksum(buf[:20])
	binary.BigEndian.PutUint16(buf[10:12], headerChecksum)

	binary.BigEndian.PutUint16(buf[20:22], srcPort)
	binary.BigEndian.PutUint16(buf[22:24], dstPort)
	binary.BigEndian.PutUint16(buf[24:26], uint16(udpLen))

	copy(buf[28:], payload)

	udpChecksum := computeUDPv4Checksum(src4, dst4, buf[20:])
	binary.BigEndian.PutUint16(buf[26:28], udpChecksum)

	return buf
}

func BuildIPv6UDPFrame(srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte) []byte {
	src16 := srcIP.To16()
	dst16 := dstIP.To16()
	if src16 == nil || dst16 == nil {
		return nil
	}

	udpLen := 8 + len(payload)
	totalLen := 40 + udpLen

	buf := make([]byte, totalLen)

	buf[0] = 0x60
	binary.BigEndian.PutUint16(buf[4:6], uint16(udpLen))
	buf[6] = 17
	buf[7] = 64
	copy(buf[8:24], src16)
	copy(buf[24:40], dst16)

	binary.BigEndian.PutUint16(buf[40:42], srcPort)
	binary.BigEndian.PutUint16(buf[42:44], dstPort)
	binary.BigEndian.PutUint16(buf[44:46], uint16(udpLen))

	copy(buf[48:], payload)

	udpChecksum := computeUDPv6Checksum(src16, dst16, buf[40:])
	binary.BigEndian.PutUint16(buf[46:48], udpChecksum)

	return buf
}

func computeIPv4HeaderChecksum(header []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(header); i += 2 {
		if i == 10 {
			continue
		}
		sum += uint32(header[i])<<8 | uint32(header[i+1])
	}
	for sum > 0xFFFF {
		sum = (sum >> 16) + (sum & 0xFFFF)
	}
	return ^uint16(sum)
}

func computeUDPv4Checksum(srcIP, dstIP, udpData []byte) uint16 {
	var sum uint32

	for i := 0; i < 4; i += 2 {
		sum += uint32(srcIP[i])<<8 | uint32(srcIP[i+1])
	}
	for i := 0; i < 4; i += 2 {
		sum += uint32(dstIP[i])<<8 | uint32(dstIP[i+1])
	}
	sum += 17
	sum += uint32(len(udpData))

	for i := 0; i+1 < len(udpData); i += 2 {
		if i == 6 {
			continue
		}
		sum += uint32(udpData[i])<<8 | uint32(udpData[i+1])
	}
	if len(udpData)%2 == 1 {
		sum += uint32(udpData[len(udpData)-1]) << 8
	}

	for sum > 0xFFFF {
		sum = (sum >> 16) + (sum & 0xFFFF)
	}
	result := ^uint16(sum)
	if result == 0 {
		return 0
	}
	return result
}

func computeUDPv6Checksum(srcIP, dstIP, udpData []byte) uint16 {
	var sum uint32

	for i := 0; i < 16; i += 2 {
		sum += uint32(srcIP[i])<<8 | uint32(srcIP[i+1])
	}
	for i := 0; i < 16; i += 2 {
		sum += uint32(dstIP[i])<<8 | uint32(dstIP[i+1])
	}
	sum += uint32(len(udpData))
	sum += 17

	for i := 0; i+1 < len(udpData); i += 2 {
		if i == 6 {
			continue
		}
		sum += uint32(udpData[i])<<8 | uint32(udpData[i+1])
	}
	if len(udpData)%2 == 1 {
		sum += uint32(udpData[len(udpData)-1]) << 8
	}

	for sum > 0xFFFF {
		sum = (sum >> 16) + (sum & 0xFFFF)
	}
	result := ^uint16(sum)
	if result == 0 {
		return 0xFFFF
	}
	return result
}
