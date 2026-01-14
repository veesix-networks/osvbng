package dhcp

import (
	"encoding/binary"
	"fmt"
	"net"
)

const (
	DHCPMinPacketSize = 236
	DHCPMagicCookie   = 0x63825363
)

type MessageType uint8

const (
	DHCPDiscover MessageType = 1
	DHCPOffer    MessageType = 2
	DHCPRequest  MessageType = 3
	DHCPDecline  MessageType = 4
	DHCPAck      MessageType = 5
	DHCPNak      MessageType = 6
	DHCPRelease  MessageType = 7
	DHCPInform   MessageType = 8
)

func (mt MessageType) String() string {
	switch mt {
	case DHCPDiscover:
		return "DHCPDISCOVER"
	case DHCPOffer:
		return "DHCPOFFER"
	case DHCPRequest:
		return "DHCPREQUEST"
	case DHCPDecline:
		return "DHCPDECLINE"
	case DHCPAck:
		return "DHCPACK"
	case DHCPNak:
		return "DHCPNAK"
	case DHCPRelease:
		return "DHCPRELEASE"
	case DHCPInform:
		return "DHCPINFORM"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", mt)
	}
}

type Packet struct {
	Op          uint8
	HType       uint8
	HLen        uint8
	Hops        uint8
	XID         uint32
	Secs        uint16
	Flags       uint16
	CIAddr      net.IP
	YIAddr      net.IP
	SIAddr      net.IP
	GIAddr      net.IP
	CHAddr      net.HardwareAddr
	MessageType MessageType
	Options     map[uint8][]byte
	RequestedIP net.IP
	ServerID    net.IP
	Hostname    string
	ClientID    []byte
	LeaseTime   uint32
	CircuitID   []byte
	RemoteID    []byte
}

func Parse(data []byte) (*Packet, error) {
	if len(data) < DHCPMinPacketSize {
		return nil, fmt.Errorf("packet too short: %d bytes", len(data))
	}

	pkt := &Packet{
		Op:      data[0],
		HType:   data[1],
		HLen:    data[2],
		Hops:    data[3],
		XID:     binary.BigEndian.Uint32(data[4:8]),
		Secs:    binary.BigEndian.Uint16(data[8:10]),
		Flags:   binary.BigEndian.Uint16(data[10:12]),
		CIAddr:  net.IP(data[12:16]),
		YIAddr:  net.IP(data[16:20]),
		SIAddr:  net.IP(data[20:24]),
		GIAddr:  net.IP(data[24:28]),
		CHAddr:  net.HardwareAddr(data[28:34]),
		Options: make(map[uint8][]byte),
	}

	if len(data) < 240 {
		return pkt, nil
	}

	magic := binary.BigEndian.Uint32(data[236:240])
	if magic != DHCPMagicCookie {
		return nil, fmt.Errorf("invalid magic cookie: 0x%x", magic)
	}

	if err := pkt.parseOptions(data[240:]); err != nil {
		return nil, fmt.Errorf("parse options: %w", err)
	}

	if msgType, ok := pkt.Options[53]; ok && len(msgType) > 0 {
		pkt.MessageType = MessageType(msgType[0])
	}

	if reqIP, ok := pkt.Options[50]; ok && len(reqIP) == 4 {
		pkt.RequestedIP = net.IP(reqIP)
	}

	if serverID, ok := pkt.Options[54]; ok && len(serverID) == 4 {
		pkt.ServerID = net.IP(serverID)
	}

	if hostname, ok := pkt.Options[12]; ok {
		pkt.Hostname = string(hostname)
	}

	if clientID, ok := pkt.Options[61]; ok {
		pkt.ClientID = clientID
	}

	if lease, ok := pkt.Options[51]; ok && len(lease) == 4 {
		pkt.LeaseTime = binary.BigEndian.Uint32(lease)
	}

	if opt82, ok := pkt.Options[82]; ok {
		pkt.parseOption82(opt82)
	}

	return pkt, nil
}

func (p *Packet) parseOptions(data []byte) error {
	i := 0
	for i < len(data) {
		if data[i] == 0 {
			i++
			continue
		}
		if data[i] == 255 {
			break
		}

		optCode := data[i]
		if i+1 >= len(data) {
			break
		}

		optLen := int(data[i+1])
		if i+2+optLen > len(data) {
			break
		}

		optData := make([]byte, optLen)
		copy(optData, data[i+2:i+2+optLen])
		p.Options[optCode] = optData

		i += 2 + optLen
	}

	return nil
}

func (p *Packet) parseOption82(data []byte) {
	i := 0
	for i < len(data) {
		if i+1 >= len(data) {
			break
		}

		subOptCode := data[i]
		subOptLen := int(data[i+1])

		if i+2+subOptLen > len(data) {
			break
		}

		subOptData := data[i+2 : i+2+subOptLen]

		switch subOptCode {
		case 1:
			p.CircuitID = subOptData
		case 2:
			p.RemoteID = subOptData
		}

		i += 2 + subOptLen
	}
}

func (p *Packet) IsRequest() bool {
	return p.Op == 1
}

func (p *Packet) IsReply() bool {
	return p.Op == 2
}
