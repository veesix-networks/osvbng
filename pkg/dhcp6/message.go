// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dhcp6

import (
	"encoding/binary"
	"errors"
	"net"
)

type MessageType uint8

const (
	MsgTypeSolicit      MessageType = 1
	MsgTypeAdvertise    MessageType = 2
	MsgTypeRequest      MessageType = 3
	MsgTypeConfirm      MessageType = 4
	MsgTypeRenew        MessageType = 5
	MsgTypeRebind       MessageType = 6
	MsgTypeReply        MessageType = 7
	MsgTypeRelease      MessageType = 8
	MsgTypeDecline      MessageType = 9
	MsgTypeReconfigure  MessageType = 10
	MsgTypeInfoRequest  MessageType = 11
	MsgTypeRelayForward MessageType = 12
	MsgTypeRelayReply   MessageType = 13
)

const (
	OptClientID            uint16 = 1
	OptServerID            uint16 = 2
	OptIANA                uint16 = 3
	OptIAAddr              uint16 = 5
	OptStatusCode          uint16 = 13
	OptRapidCommit         uint16 = 14
	OptInterfaceID         uint16 = 18
	OptDNSServers          uint16 = 23
	OptDomainList          uint16 = 24
	OptIAPD                uint16 = 25
	OptIAPrefix            uint16 = 26
	OptRemoteID            uint16 = 37
	OptClientLinkLayerAddr uint16 = 79
	OptRelayMsg            uint16 = 9
)

var (
	ErrTooShort       = errors.New("dhcpv6: message too short")
	ErrInvalidRelay   = errors.New("dhcpv6: invalid relay message")
	ErrNoRelayMessage = errors.New("dhcpv6: no relay message option found")
)

type Message struct {
	MsgType       MessageType
	TransactionID [3]byte
	Options       Options
	Raw           []byte
}

type Options struct {
	ClientID            []byte
	ServerID            []byte
	IANA                *IANAOption
	IAPD                *IAPDOption
	DNS                 []net.IP
	InterfaceID         []byte
	RemoteID            []byte
	ClientLinkLayerAddr []byte
	RapidCommit         bool
	StatusCode          *StatusCodeOption
}

type StatusCodeOption struct {
	Code    uint16
	Message string
}

type IANAOption struct {
	IAID          uint32
	T1            uint32
	T2            uint32
	Address       net.IP
	PreferredTime uint32
	ValidTime     uint32
}

type IAPDOption struct {
	IAID          uint32
	T1            uint32
	T2            uint32
	PrefixLen     uint8
	Prefix        net.IP
	PreferredTime uint32
	ValidTime     uint32
}

type RelayInfo struct {
	HopCount            uint8
	LinkAddr            net.IP
	PeerAddr            net.IP
	InterfaceID         []byte
	RemoteID            []byte
	ClientLinkLayerAddr []byte
}

func ParseMessage(data []byte) (*Message, error) {
	if len(data) < 4 {
		return nil, ErrTooShort
	}
	msg := &Message{
		MsgType: MessageType(data[0]),
		Raw:     data,
	}
	copy(msg.TransactionID[:], data[1:4])
	msg.Options = ParseOptions(data[4:])
	return msg, nil
}

func ParseOptions(data []byte) Options {
	var opts Options
	for len(data) >= 4 {
		code := binary.BigEndian.Uint16(data[0:2])
		length := binary.BigEndian.Uint16(data[2:4])
		if len(data) < int(4+length) {
			break
		}
		optData := data[4 : 4+length]
		switch code {
		case OptClientID:
			opts.ClientID = optData
		case OptServerID:
			opts.ServerID = optData
		case OptIANA:
			opts.IANA = parseIANA(optData)
		case OptIAPD:
			opts.IAPD = parseIAPD(optData)
		case OptDNSServers:
			opts.DNS = parseDNSServers(optData)
		case OptInterfaceID:
			opts.InterfaceID = optData
		case OptRemoteID:
			opts.RemoteID = optData
		case OptClientLinkLayerAddr:
			opts.ClientLinkLayerAddr = optData
		case OptRapidCommit:
			opts.RapidCommit = true
		case OptStatusCode:
			if len(optData) >= 2 {
				opts.StatusCode = &StatusCodeOption{
					Code:    binary.BigEndian.Uint16(optData[0:2]),
					Message: string(optData[2:]),
				}
			}
		}
		data = data[4+length:]
	}
	return opts
}

func parseIANA(data []byte) *IANAOption {
	if len(data) < 12 {
		return nil
	}
	opt := &IANAOption{
		IAID: binary.BigEndian.Uint32(data[0:4]),
		T1:   binary.BigEndian.Uint32(data[4:8]),
		T2:   binary.BigEndian.Uint32(data[8:12]),
	}
	sub := data[12:]
	for len(sub) >= 4 {
		subCode := binary.BigEndian.Uint16(sub[0:2])
		subLen := binary.BigEndian.Uint16(sub[2:4])
		if len(sub) < int(4+subLen) {
			break
		}
		if subCode == OptIAAddr && subLen >= 24 {
			opt.Address = net.IP(sub[4:20])
			opt.PreferredTime = binary.BigEndian.Uint32(sub[20:24])
			opt.ValidTime = binary.BigEndian.Uint32(sub[24:28])
		}
		sub = sub[4+subLen:]
	}
	return opt
}

func parseIAPD(data []byte) *IAPDOption {
	if len(data) < 12 {
		return nil
	}
	opt := &IAPDOption{
		IAID: binary.BigEndian.Uint32(data[0:4]),
		T1:   binary.BigEndian.Uint32(data[4:8]),
		T2:   binary.BigEndian.Uint32(data[8:12]),
	}
	sub := data[12:]
	for len(sub) >= 4 {
		subCode := binary.BigEndian.Uint16(sub[0:2])
		subLen := binary.BigEndian.Uint16(sub[2:4])
		if len(sub) < int(4+subLen) {
			break
		}
		if subCode == OptIAPrefix && subLen >= 25 {
			opt.PreferredTime = binary.BigEndian.Uint32(sub[4:8])
			opt.ValidTime = binary.BigEndian.Uint32(sub[8:12])
			opt.PrefixLen = sub[12]
			opt.Prefix = net.IP(sub[13:29])
		}
		sub = sub[4+subLen:]
	}
	return opt
}

func parseDNSServers(data []byte) []net.IP {
	if len(data) < 16 {
		return nil
	}
	var servers []net.IP
	for i := 0; i+16 <= len(data); i += 16 {
		ip := make(net.IP, 16)
		copy(ip, data[i:i+16])
		servers = append(servers, ip)
	}
	return servers
}

func UnwrapRelay(data []byte) (*Message, *RelayInfo) {
	if len(data) < 34 || MessageType(data[0]) != MsgTypeRelayForward {
		return nil, nil
	}

	info := &RelayInfo{
		HopCount: data[1],
		LinkAddr: net.IP(data[2:18]),
		PeerAddr: net.IP(data[18:34]),
	}

	relayOpts := ParseOptions(data[34:])
	info.InterfaceID = relayOpts.InterfaceID
	info.RemoteID = relayOpts.RemoteID
	info.ClientLinkLayerAddr = relayOpts.ClientLinkLayerAddr

	for offset := 34; offset+4 <= len(data); {
		code := binary.BigEndian.Uint16(data[offset : offset+2])
		length := binary.BigEndian.Uint16(data[offset+2 : offset+4])
		if offset+4+int(length) > len(data) {
			break
		}
		if code == OptRelayMsg {
			innerData := data[offset+4 : offset+4+int(length)]
			if len(innerData) > 0 && MessageType(innerData[0]) == MsgTypeRelayForward {
				return UnwrapRelay(innerData)
			}
			msg, err := ParseMessage(innerData)
			if err != nil {
				return nil, info
			}
			return msg, info
		}
		offset += 4 + int(length)
	}
	return nil, info
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
	clone.Options = m.Options.DeepCopy()
	return &clone
}

func (o Options) DeepCopy() Options {
	clone := o
	if o.ClientID != nil {
		clone.ClientID = append([]byte(nil), o.ClientID...)
	}
	if o.ServerID != nil {
		clone.ServerID = append([]byte(nil), o.ServerID...)
	}
	if o.InterfaceID != nil {
		clone.InterfaceID = append([]byte(nil), o.InterfaceID...)
	}
	if o.RemoteID != nil {
		clone.RemoteID = append([]byte(nil), o.RemoteID...)
	}
	if o.ClientLinkLayerAddr != nil {
		clone.ClientLinkLayerAddr = append([]byte(nil), o.ClientLinkLayerAddr...)
	}
	if o.DNS != nil {
		clone.DNS = make([]net.IP, len(o.DNS))
		for i, ip := range o.DNS {
			clone.DNS[i] = append(net.IP(nil), ip...)
		}
	}
	if o.IANA != nil {
		c := *o.IANA
		if o.IANA.Address != nil {
			c.Address = append(net.IP(nil), o.IANA.Address...)
		}
		clone.IANA = &c
	}
	if o.IAPD != nil {
		c := *o.IAPD
		if o.IAPD.Prefix != nil {
			c.Prefix = append(net.IP(nil), o.IAPD.Prefix...)
		}
		clone.IAPD = &c
	}
	if o.StatusCode != nil {
		c := *o.StatusCode
		clone.StatusCode = &c
	}
	return clone
}
