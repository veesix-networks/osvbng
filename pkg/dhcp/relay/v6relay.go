// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package relay

import (
	"encoding/binary"
	"fmt"
	"net"
)

const (
	DHCPv6MsgRelayForward = 12
	DHCPv6MsgRelayReply   = 13

	DHCPv6OptRelayMessage  = 9
	DHCPv6OptInterfaceID   = 18
	DHCPv6OptRemoteID      = 37
	DHCPv6OptSubscriberID  = 38

	DHCPv6RelayHeaderLen = 34 // msg-type(1) + hop-count(1) + link-addr(16) + peer-addr(16)
)

type RelayForwardParams struct {
	HopCount         uint8
	LinkAddress      net.IP
	PeerAddress      net.IP
	InterfaceID      []byte
	RemoteID         []byte
	EnterpriseNumber uint32
	SubscriberID     []byte
}

// BuildRelayForward wraps a DHCPv6 client message in a Relay-Forward envelope.
func BuildRelayForward(clientMsg []byte, p *RelayForwardParams) []byte {
	relayMsgOptLen := 4 + len(clientMsg) // option-code(2) + option-len(2) + data

	totalOpts := relayMsgOptLen
	if len(p.InterfaceID) > 0 {
		totalOpts += 4 + len(p.InterfaceID)
	}
	if len(p.RemoteID) > 0 {
		totalOpts += 4 + 4 + len(p.RemoteID)
	}
	if len(p.SubscriberID) > 0 {
		totalOpts += 4 + len(p.SubscriberID)
	}

	buf := make([]byte, DHCPv6RelayHeaderLen+totalOpts)
	buf[0] = DHCPv6MsgRelayForward
	buf[1] = p.HopCount

	linkAddr := p.LinkAddress.To16()
	if linkAddr == nil {
		linkAddr = net.IPv6zero
	}
	copy(buf[2:18], linkAddr)

	peerAddr := p.PeerAddress.To16()
	if peerAddr == nil {
		peerAddr = net.IPv6zero
	}
	copy(buf[18:34], peerAddr)

	offset := DHCPv6RelayHeaderLen

	if len(p.InterfaceID) > 0 {
		binary.BigEndian.PutUint16(buf[offset:], DHCPv6OptInterfaceID)
		binary.BigEndian.PutUint16(buf[offset+2:], uint16(len(p.InterfaceID)))
		copy(buf[offset+4:], p.InterfaceID)
		offset += 4 + len(p.InterfaceID)
	}

	if len(p.RemoteID) > 0 {
		binary.BigEndian.PutUint16(buf[offset:], DHCPv6OptRemoteID)
		binary.BigEndian.PutUint16(buf[offset+2:], uint16(4+len(p.RemoteID)))
		binary.BigEndian.PutUint32(buf[offset+4:], p.EnterpriseNumber)
		copy(buf[offset+8:], p.RemoteID)
		offset += 4 + 4 + len(p.RemoteID)
	}

	if len(p.SubscriberID) > 0 {
		binary.BigEndian.PutUint16(buf[offset:], DHCPv6OptSubscriberID)
		binary.BigEndian.PutUint16(buf[offset+2:], uint16(len(p.SubscriberID)))
		copy(buf[offset+4:], p.SubscriberID)
		offset += 4 + len(p.SubscriberID)
	}

	// Relay-Message option (MUST be last for easier parsing)
	binary.BigEndian.PutUint16(buf[offset:], DHCPv6OptRelayMessage)
	binary.BigEndian.PutUint16(buf[offset+2:], uint16(len(clientMsg)))
	copy(buf[offset+4:], clientMsg)

	return buf
}

// UnwrapRelayReply extracts the inner message from a Relay-Reply.
// Returns the inner message and the relay options.
func UnwrapRelayReply(pkt []byte) (innerMsg []byte, err error) {
	if len(pkt) < DHCPv6RelayHeaderLen {
		return nil, fmt.Errorf("packet too short for relay-reply: %d", len(pkt))
	}
	if pkt[0] != DHCPv6MsgRelayReply {
		return nil, fmt.Errorf("not a relay-reply: msg-type %d", pkt[0])
	}

	inner := extractRelayMessage(pkt)
	if inner == nil {
		return nil, fmt.Errorf("no relay-message option in relay-reply")
	}

	return inner, nil
}

// extractRelayMessage finds the Relay-Message option (9) in a relay message
// and returns its contents (the inner message).
func extractRelayMessage(pkt []byte) []byte {
	if len(pkt) < DHCPv6RelayHeaderLen {
		return nil
	}

	i := DHCPv6RelayHeaderLen
	for i+4 <= len(pkt) {
		optCode := binary.BigEndian.Uint16(pkt[i:])
		optLen := binary.BigEndian.Uint16(pkt[i+2:])

		if i+4+int(optLen) > len(pkt) {
			return nil
		}

		if optCode == DHCPv6OptRelayMessage {
			inner := make([]byte, optLen)
			copy(inner, pkt[i+4:i+4+int(optLen)])
			return inner
		}

		i += 4 + int(optLen)
	}
	return nil
}

// GetRelayTransactionID extracts the transaction ID from a relay message
// by finding the inner client message.
func GetRelayTransactionID(pkt []byte) ([3]byte, bool) {
	var txnID [3]byte
	inner := extractRelayMessage(pkt)
	if inner == nil || len(inner) < 4 {
		return txnID, false
	}
	copy(txnID[:], inner[1:4])
	return txnID, true
}
