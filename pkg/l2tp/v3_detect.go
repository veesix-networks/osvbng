// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import "encoding/binary"

// IsL2TPv3 returns true if the first two bytes of the L2TP datagram
// encode protocol version 3 (RFC 3931). The version field lives in
// the low nibble of the second header byte. Cheap to call on every
// inbound packet before doing a full parse.
func IsL2TPv3(headerBytes []byte) bool {
	if len(headerBytes) < 2 {
		return false
	}
	flags := binary.BigEndian.Uint16(headerBytes[:2])
	return uint8(flags&verMask) == Version3
}

// BuildV3RejectStopCCN builds a complete L2TPv2 StopCCN that informs a
// v3-speaking peer we do not support v3. Per RFC 2661 §4.4.2 the
// Result Code AVP carries Result Code 5 ("protocol version not
// supported"), Error Code 0x0100 ("highest version supported = Version
// 1, Revision 0" — the ze guide §5.3 / spec-finalize finding G2
// reading of RFC 2661's Error Code semantics for the version-mismatch
// case).
//
// `localTunnelID` is the ID we assigned for this exchange (returned to
// peer in the Assigned Tunnel ID AVP). `peerTunnelID` is the ID we put
// in the L2TP header's Tunnel ID field (the value the v3 peer sent us
// in its SCCRQ); when peerTunnelID is zero we emit a header with
// Tunnel ID 0 which is acceptable for a one-shot StopCCN reply.
//
// The returned slice contains an entire L2TP control message ready to
// hand to UDP. The Length field is filled in correctly.
func BuildV3RejectStopCCN(localTunnelID, peerTunnelID uint16) []byte {
	// Body = Message Type AVP (StopCCN) + Result Code AVP +
	//        Assigned Tunnel ID AVP.
	body := make([]byte, 0, 32)
	body = appendMessageTypeAVP(body, MsgTypeStopCCN)
	body = appendResultCodeAVP(body, ResultStopVersionUnsupported, 0x0100, nil)
	body = appendAssignedTunnelIDAVP(body, localTunnelID)

	h := NewControl(peerTunnelID, 0, 0, 0)
	out := h.AppendTo(make([]byte, 0, h.HeaderLenBytes()+len(body)), len(body))
	out = append(out, body...)
	return out
}
