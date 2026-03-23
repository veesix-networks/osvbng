// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dhcp4

import (
	"encoding/binary"
	"errors"
	"net"
)

type MessageType uint8

const (
	MsgTypeDiscover MessageType = 1
	MsgTypeOffer    MessageType = 2
	MsgTypeRequest  MessageType = 3
	MsgTypeDecline  MessageType = 4
	MsgTypeAck      MessageType = 5
	MsgTypeNak      MessageType = 6
	MsgTypeRelease  MessageType = 7
	MsgTypeInform   MessageType = 8
)

const (
	OptSubnetMask   uint8 = 1
	OptRouter       uint8 = 3
	OptDNS          uint8 = 6
	OptHostname     uint8 = 12
	OptDomainName   uint8 = 15
	OptRequestedIP  uint8 = 50
	OptLeaseTime    uint8 = 51
	OptMessageType  uint8 = 53
	OptServerID     uint8 = 54
	OptParamRequest uint8 = 55
	OptClientID     uint8 = 61
	OptOption82     uint8 = 82
	OptEnd          uint8 = 255
)

const DHCPv4HeaderLen = 236

var (
	ErrTooShort = errors.New("dhcpv4: message too short")
)

type Message struct {
	Op            uint8
	HType         uint8
	HLen          uint8
	Hops          uint8
	XID           uint32
	Secs          uint16
	Flags         uint16
	ClientIP      net.IP
	YourIP        net.IP
	ServerIP      net.IP
	GatewayIP     net.IP
	ClientHWAddr  net.HardwareAddr
	ServerName    [64]byte
	BootFileName  [128]byte
	Options       Options
	Raw           []byte
}

type Options struct {
	MessageType MessageType
	ServerID    net.IP
	RequestedIP net.IP
	Hostname    string
	ClientID    []byte
	LeaseTime   uint32
	SubnetMask  net.IPMask
	Router      net.IP
	DNS         []net.IP
	Option82    []byte
}

func ParseMessage(data []byte) (*Message, error) {
	if len(data) < DHCPv4HeaderLen+4 {
		return nil, ErrTooShort
	}

	msg := &Message{
		Op:    data[0],
		HType: data[1],
		HLen:  data[2],
		Hops:  data[3],
		XID:   binary.BigEndian.Uint32(data[4:8]),
		Secs:  binary.BigEndian.Uint16(data[8:10]),
		Flags: binary.BigEndian.Uint16(data[10:12]),
		Raw:   data,
	}

	msg.ClientIP = net.IP(data[12:16])
	msg.YourIP = net.IP(data[16:20])
	msg.ServerIP = net.IP(data[20:24])
	msg.GatewayIP = net.IP(data[24:28])

	hwLen := int(msg.HLen)
	if hwLen > 16 {
		hwLen = 16
	}
	msg.ClientHWAddr = net.HardwareAddr(data[28 : 28+hwLen])

	copy(msg.ServerName[:], data[44:108])
	copy(msg.BootFileName[:], data[108:236])

	magic := binary.BigEndian.Uint32(data[236:240])
	if magic != 0x63825363 {
		return msg, nil
	}

	msg.Options = parseOptions(data[240:])
	return msg, nil
}

func parseOptions(data []byte) Options {
	var opts Options
	for len(data) >= 2 {
		optType := data[0]
		if optType == OptEnd {
			break
		}
		if optType == 0 {
			data = data[1:]
			continue
		}
		optLen := int(data[1])
		if len(data) < 2+optLen {
			break
		}
		optData := data[2 : 2+optLen]

		switch optType {
		case OptMessageType:
			if optLen >= 1 {
				opts.MessageType = MessageType(optData[0])
			}
		case OptServerID:
			if optLen >= 4 {
				opts.ServerID = net.IP(optData[:4])
			}
		case OptRequestedIP:
			if optLen >= 4 {
				opts.RequestedIP = net.IP(optData[:4])
			}
		case OptHostname:
			opts.Hostname = string(optData)
		case OptClientID:
			opts.ClientID = optData
		case OptLeaseTime:
			if optLen >= 4 {
				opts.LeaseTime = binary.BigEndian.Uint32(optData[:4])
			}
		case OptSubnetMask:
			if optLen >= 4 {
				opts.SubnetMask = net.IPMask(optData[:4])
			}
		case OptRouter:
			if optLen >= 4 {
				opts.Router = net.IP(optData[:4])
			}
		case OptDNS:
			for i := 0; i+4 <= optLen; i += 4 {
				ip := make(net.IP, 4)
				copy(ip, optData[i:i+4])
				opts.DNS = append(opts.DNS, ip)
			}
		case OptOption82:
			opts.Option82 = optData
		}
		data = data[2+optLen:]
	}
	return opts
}

func (m *Message) DeepCopy() *Message {
	if m == nil {
		return nil
	}
	clone := *m
	if m.Raw != nil {
		clone.Raw = make([]byte, len(m.Raw))
		copy(clone.Raw, m.Raw)
	}
	if m.ClientIP != nil {
		clone.ClientIP = append(net.IP(nil), m.ClientIP...)
	}
	if m.YourIP != nil {
		clone.YourIP = append(net.IP(nil), m.YourIP...)
	}
	if m.ServerIP != nil {
		clone.ServerIP = append(net.IP(nil), m.ServerIP...)
	}
	if m.GatewayIP != nil {
		clone.GatewayIP = append(net.IP(nil), m.GatewayIP...)
	}
	if m.ClientHWAddr != nil {
		clone.ClientHWAddr = append(net.HardwareAddr(nil), m.ClientHWAddr...)
	}
	clone.Options = m.Options.DeepCopy()
	return &clone
}

func (o Options) DeepCopy() Options {
	clone := o
	if o.ServerID != nil {
		clone.ServerID = append(net.IP(nil), o.ServerID...)
	}
	if o.RequestedIP != nil {
		clone.RequestedIP = append(net.IP(nil), o.RequestedIP...)
	}
	if o.ClientID != nil {
		clone.ClientID = append([]byte(nil), o.ClientID...)
	}
	if o.SubnetMask != nil {
		clone.SubnetMask = append(net.IPMask(nil), o.SubnetMask...)
	}
	if o.Router != nil {
		clone.Router = append(net.IP(nil), o.Router...)
	}
	if o.DNS != nil {
		clone.DNS = make([]net.IP, len(o.DNS))
		for i, ip := range o.DNS {
			clone.DNS[i] = append(net.IP(nil), ip...)
		}
	}
	if o.Option82 != nil {
		clone.Option82 = append([]byte(nil), o.Option82...)
	}
	return clone
}
