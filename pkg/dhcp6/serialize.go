// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dhcp6

import (
	"encoding/binary"
	"net"
)

type Response struct {
	MsgType       MessageType
	TransactionID [3]byte
	ClientID      []byte
	ServerID      []byte
	IANA          *IANAOption
	IAPD          *IAPDOption
	DNS           []net.IP
	StatusCode    *StatusCodeOption
}

func (r *Response) Serialize() []byte {
	size := 4
	size += optionLen(OptClientID, len(r.ClientID))
	size += optionLen(OptServerID, len(r.ServerID))
	if r.IANA != nil && r.IANA.Address != nil {
		size += optionLen(OptIANA, 12+optionLen(OptIAAddr, 24))
	}
	if r.IAPD != nil && r.IAPD.Prefix != nil {
		size += optionLen(OptIAPD, 12+optionLen(OptIAPrefix, 25))
	}
	if len(r.DNS) > 0 {
		size += optionLen(OptDNSServers, len(r.DNS)*16)
	}
	if r.StatusCode != nil {
		size += optionLen(OptStatusCode, 2+len(r.StatusCode.Message))
	}

	buf := make([]byte, size)
	buf[0] = byte(r.MsgType)
	copy(buf[1:4], r.TransactionID[:])
	offset := 4

	offset = writeOption(buf, offset, OptClientID, r.ClientID)
	offset = writeOption(buf, offset, OptServerID, r.ServerID)

	if r.IANA != nil && r.IANA.Address != nil {
		ianaPayload := buildIANAPayload(r.IANA)
		offset = writeOption(buf, offset, OptIANA, ianaPayload)
	}

	if r.IAPD != nil && r.IAPD.Prefix != nil {
		iapdPayload := buildIAPDPayload(r.IAPD)
		offset = writeOption(buf, offset, OptIAPD, iapdPayload)
	}

	if len(r.DNS) > 0 {
		dnsPayload := make([]byte, len(r.DNS)*16)
		for i, ip := range r.DNS {
			copy(dnsPayload[i*16:], ip.To16())
		}
		offset = writeOption(buf, offset, OptDNSServers, dnsPayload)
	}

	if r.StatusCode != nil {
		statusPayload := make([]byte, 2+len(r.StatusCode.Message))
		binary.BigEndian.PutUint16(statusPayload[0:2], r.StatusCode.Code)
		copy(statusPayload[2:], r.StatusCode.Message)
		offset = writeOption(buf, offset, OptStatusCode, statusPayload)
	}

	return buf[:offset]
}

func buildIANAPayload(opt *IANAOption) []byte {
	subOptLen := optionLen(OptIAAddr, 24)
	payload := make([]byte, 12+subOptLen)
	binary.BigEndian.PutUint32(payload[0:4], opt.IAID)
	binary.BigEndian.PutUint32(payload[4:8], opt.T1)
	binary.BigEndian.PutUint32(payload[8:12], opt.T2)

	binary.BigEndian.PutUint16(payload[12:14], OptIAAddr)
	binary.BigEndian.PutUint16(payload[14:16], 24)
	copy(payload[16:32], opt.Address.To16())
	binary.BigEndian.PutUint32(payload[32:36], opt.PreferredTime)
	binary.BigEndian.PutUint32(payload[36:40], opt.ValidTime)

	return payload
}

func buildIAPDPayload(opt *IAPDOption) []byte {
	subOptLen := optionLen(OptIAPrefix, 25)
	payload := make([]byte, 12+subOptLen)
	binary.BigEndian.PutUint32(payload[0:4], opt.IAID)
	binary.BigEndian.PutUint32(payload[4:8], opt.T1)
	binary.BigEndian.PutUint32(payload[8:12], opt.T2)

	binary.BigEndian.PutUint16(payload[12:14], OptIAPrefix)
	binary.BigEndian.PutUint16(payload[14:16], 25)
	binary.BigEndian.PutUint32(payload[16:20], opt.PreferredTime)
	binary.BigEndian.PutUint32(payload[20:24], opt.ValidTime)
	payload[24] = opt.PrefixLen
	copy(payload[25:41], opt.Prefix.To16())

	return payload
}

func optionLen(code uint16, dataLen int) int {
	_ = code
	return 4 + dataLen
}

func writeOption(buf []byte, offset int, code uint16, data []byte) int {
	binary.BigEndian.PutUint16(buf[offset:offset+2], code)
	binary.BigEndian.PutUint16(buf[offset+2:offset+4], uint16(len(data)))
	copy(buf[offset+4:], data)
	return offset + 4 + len(data)
}
