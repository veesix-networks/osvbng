package arp

import (
	"encoding/binary"
	"fmt"
	"net"
)

const (
	OpRequest = 1
	OpReply   = 2
)

type Packet struct {
	Operation  uint16
	SenderMAC  net.HardwareAddr
	SenderIP   net.IP
	TargetMAC  net.HardwareAddr
	TargetIP   net.IP
}

func Parse(data []byte) (*Packet, error) {
	if len(data) < 28 {
		return nil, fmt.Errorf("packet too short: %d bytes", len(data))
	}

	htype := binary.BigEndian.Uint16(data[0:2])
	ptype := binary.BigEndian.Uint16(data[2:4])
	hlen := data[4]
	plen := data[5]

	if htype != 1 || ptype != 0x0800 || hlen != 6 || plen != 4 {
		return nil, fmt.Errorf("unsupported ARP format")
	}

	operation := binary.BigEndian.Uint16(data[6:8])

	senderMAC := make(net.HardwareAddr, 6)
	copy(senderMAC, data[8:14])

	senderIP := net.IPv4(data[14], data[15], data[16], data[17])

	targetMAC := make(net.HardwareAddr, 6)
	copy(targetMAC, data[18:24])

	targetIP := net.IPv4(data[24], data[25], data[26], data[27])

	return &Packet{
		Operation: operation,
		SenderMAC: senderMAC,
		SenderIP:  senderIP,
		TargetMAC: targetMAC,
		TargetIP:  targetIP,
	}, nil
}

func BuildReply(request *Packet, replyMAC net.HardwareAddr) []byte {
	reply := make([]byte, 28)

	binary.BigEndian.PutUint16(reply[0:2], 1)
	binary.BigEndian.PutUint16(reply[2:4], 0x0800)
	reply[4] = 6
	reply[5] = 4
	binary.BigEndian.PutUint16(reply[6:8], OpReply)

	copy(reply[8:14], replyMAC)
	copy(reply[14:18], request.TargetIP.To4())

	copy(reply[18:24], request.SenderMAC)
	copy(reply[24:28], request.SenderIP.To4())

	return reply
}
