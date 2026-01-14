package dhcp

import (
	"encoding/binary"
	"net"
)

// We should convert to use gopacket at some point...
func BuildUDPPacket(srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte) []byte {
	totalLen := 20 + 8 + len(payload)
	packet := make([]byte, totalLen)

	ipHeader := packet[:20]
	ipHeader[0] = 0x45
	binary.BigEndian.PutUint16(ipHeader[2:4], uint16(totalLen))
	ipHeader[8] = 64
	ipHeader[9] = 17
	copy(ipHeader[12:16], srcIP.To4())
	copy(ipHeader[16:20], dstIP.To4())

	ipChecksum := calculateChecksum(ipHeader)
	binary.BigEndian.PutUint16(ipHeader[10:12], ipChecksum)

	udpHeader := packet[20:28]
	binary.BigEndian.PutUint16(udpHeader[0:2], srcPort)
	binary.BigEndian.PutUint16(udpHeader[2:4], dstPort)
	binary.BigEndian.PutUint16(udpHeader[4:6], uint16(8+len(payload)))
	udpChecksum := calculateUDPChecksum(srcIP.To4(), dstIP.To4(), udpHeader, payload)
	binary.BigEndian.PutUint16(udpHeader[6:8], udpChecksum)

	copy(packet[28:], payload)

	return packet
}

func calculateChecksum(data []byte) uint16 {
	sum := uint32(0)
	for i := 0; i < len(data); i += 2 {
		if i+1 < len(data) {
			sum += uint32(data[i])<<8 | uint32(data[i+1])
		} else {
			sum += uint32(data[i]) << 8
		}
	}
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func calculateUDPChecksum(srcIP, dstIP []byte, udpHeader, payload []byte) uint16 {
	pseudoHeader := make([]byte, 12)
	copy(pseudoHeader[0:4], srcIP)
	copy(pseudoHeader[4:8], dstIP)
	pseudoHeader[8] = 0
	pseudoHeader[9] = 17
	udpLen := uint16(len(udpHeader) + len(payload))
	binary.BigEndian.PutUint16(pseudoHeader[10:12], udpLen)

	data := append(pseudoHeader, udpHeader...)
	data = append(data, payload...)

	sum := uint32(0)
	for i := 0; i < len(data); i += 2 {
		if i+1 < len(data) {
			sum += uint32(data[i])<<8 | uint32(data[i+1])
		} else {
			sum += uint32(data[i]) << 8
		}
	}
	for sum > 0xffff {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	checksum := ^uint16(sum)

	if checksum == 0 {
		checksum = 0xFFFF
	}

	return checksum
}
