// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import "encoding/binary"

// AVP attribute types per RFC 2661 §4.4 and successors. All entries
// are listed even when unused so the parser can distinguish "known
// but ignored" from "unknown mandatory" — the latter is the only
// case that warrants a StopCCN/CDN per §4.1.
const (
	AVPMessageType          uint16 = 0  // RFC 2661 §4.4.1
	AVPResultCode           uint16 = 1  // §4.4.2
	AVPProtocolVersion      uint16 = 2  // §4.4.3
	AVPFramingCapabilities  uint16 = 3  // §4.4.4
	AVPBearerCapabilities   uint16 = 4  // §4.4.4
	AVPTieBreaker           uint16 = 5  // §4.4.5
	AVPFirmwareRevision     uint16 = 6  // §4.4.6
	AVPHostName             uint16 = 7  // §4.4.7
	AVPVendorName           uint16 = 8  // §4.4.8
	AVPAssignedTunnelID     uint16 = 9  // §4.4.9
	AVPReceiveWindowSize    uint16 = 10 // §4.4.10
	AVPChallenge            uint16 = 11 // §4.4.11
	AVPCauseCode            uint16 = 12 // Q.931 cause
	AVPChallengeResponse    uint16 = 13 // §4.4.11
	AVPAssignedSessionID    uint16 = 14 // §4.4.12
	AVPCallSerialNumber     uint16 = 15 // §4.4.13
	AVPMinimumBPS           uint16 = 16 // §4.4.14
	AVPMaximumBPS           uint16 = 17 // §4.4.14
	AVPBearerType           uint16 = 18 // §4.4.15
	AVPFramingType          uint16 = 19 // §4.4.16
	AVPCalledNumber         uint16 = 21 // §4.4.17
	AVPCallingNumber        uint16 = 22 // §4.4.18
	AVPSubAddress           uint16 = 23 // §4.4.19
	AVPTxConnectSpeed       uint16 = 24 // §4.4.20
	AVPPhysicalChannelID    uint16 = 25 // §4.4.21
	AVPInitialRxLCPConfReq  uint16 = 26 // §4.4.22
	AVPLastSentLCPConfReq   uint16 = 27 // §4.4.22
	AVPLastRecvLCPConfReq   uint16 = 28 // §4.4.22
	AVPProxyAuthenType      uint16 = 29 // §4.4.23
	AVPProxyAuthenName      uint16 = 30 // §4.4.24
	AVPProxyAuthenChallenge uint16 = 31 // §4.4.25
	AVPProxyAuthenID        uint16 = 32 // §4.4.26
	AVPProxyAuthenResponse  uint16 = 33 // §4.4.27
	AVPCallErrors           uint16 = 34 // §4.4.28 — zero-padded in ICCN per spec-finalize G2
	AVPACCM                 uint16 = 35 // §4.4.29 — zero for sync framing
	AVPRandomVector         uint16 = 36 // §4.3
	AVPPrivateGroupID       uint16 = 37 // §4.4.30
	AVPRxConnectSpeed       uint16 = 38 // §4.4.20
	AVPSequencingRequired   uint16 = 39 // §4.4.31
)

// Message Type values per RFC 2661 §4.4.1.
const (
	MsgTypeSCCRQ   uint16 = 1  // Start-Control-Connection-Request
	MsgTypeSCCRP   uint16 = 2  // Start-Control-Connection-Reply
	MsgTypeSCCCN   uint16 = 3  // Start-Control-Connection-Connected
	MsgTypeStopCCN uint16 = 4  // Stop-Control-Connection-Notification
	MsgTypeHello   uint16 = 6  // Hello
	MsgTypeOCRQ    uint16 = 7  // Outgoing-Call-Request (out of v1 scope)
	MsgTypeOCRP    uint16 = 8  // Outgoing-Call-Reply
	MsgTypeOCCN    uint16 = 9  // Outgoing-Call-Connected
	MsgTypeICRQ    uint16 = 10 // Incoming-Call-Request
	MsgTypeICRP    uint16 = 11 // Incoming-Call-Reply
	MsgTypeICCN    uint16 = 12 // Incoming-Call-Connected
	MsgTypeCDN     uint16 = 14 // Call-Disconnect-Notify
	MsgTypeWEN     uint16 = 15 // WAN-Error-Notify
	MsgTypeSLI     uint16 = 16 // Set-Link-Info
)

// MessageTypeName returns the short name for a message type or "" if
// the type is unknown. Used in traces and error messages.
func MessageTypeName(t uint16) string {
	switch t {
	case MsgTypeSCCRQ:
		return "SCCRQ"
	case MsgTypeSCCRP:
		return "SCCRP"
	case MsgTypeSCCCN:
		return "SCCCN"
	case MsgTypeStopCCN:
		return "StopCCN"
	case MsgTypeHello:
		return "Hello"
	case MsgTypeOCRQ:
		return "OCRQ"
	case MsgTypeOCRP:
		return "OCRP"
	case MsgTypeOCCN:
		return "OCCN"
	case MsgTypeICRQ:
		return "ICRQ"
	case MsgTypeICRP:
		return "ICRP"
	case MsgTypeICCN:
		return "ICCN"
	case MsgTypeCDN:
		return "CDN"
	case MsgTypeWEN:
		return "WEN"
	case MsgTypeSLI:
		return "SLI"
	}
	return ""
}

// Framing & Bearer capabilities bit fields (RFC 2661 §4.4.4).
const (
	FramingAsync uint32 = 1 << 0
	FramingSync  uint32 = 1 << 1

	BearerAnalog  uint32 = 1 << 0
	BearerDigital uint32 = 1 << 1
)

// IETF AVPs use VendorID 0.
const VendorIETF uint16 = 0

// Below are typed constructors for the AVPs we emit. Each returns the
// dst slice extended by one AVP. All IETF AVPs are emitted with
// VendorID=0; Mandatory=true is the default per RFC 2661 §4.4 (most
// AVPs are mandatory).

// BuildHello returns the AVP body of an L2TPv2 Hello control message
// (RFC 2661 §6.5) — just the Message Type AVP. The L2TP header (with
// Ns/Nr) is added by the control channel.
func BuildHello() []byte {
	return appendMessageTypeAVP(nil, MsgTypeHello)
}

func appendMessageTypeAVP(dst []byte, msgType uint16) []byte {
	var v [2]byte
	binary.BigEndian.PutUint16(v[:], msgType)
	return AppendAVP(dst, true, false, VendorIETF, AVPMessageType, v[:])
}

func appendResultCodeAVP(dst []byte, rc ResultCode, ec ErrorCode, msg []byte) []byte {
	// Layout: result-code(2) [+ error-code(2) [+ optional ASCII message]].
	// Per RFC 2661 §4.4.2 only result-code is mandatory; if error-code
	// is non-zero or a message is provided, they follow in order.
	val := make([]byte, 0, 4+len(msg))
	val = binary.BigEndian.AppendUint16(val, uint16(rc))
	if ec != 0 || len(msg) > 0 {
		val = binary.BigEndian.AppendUint16(val, uint16(ec))
		val = append(val, msg...)
	}
	return AppendAVP(dst, true, false, VendorIETF, AVPResultCode, val)
}

func appendProtocolVersionAVP(dst []byte, version, revision uint8) []byte {
	v := []byte{version, revision}
	return AppendAVP(dst, true, false, VendorIETF, AVPProtocolVersion, v)
}

func appendFramingCapabilitiesAVP(dst []byte, caps uint32) []byte {
	var v [4]byte
	binary.BigEndian.PutUint32(v[:], caps)
	return AppendAVP(dst, true, false, VendorIETF, AVPFramingCapabilities, v[:])
}

func appendBearerCapabilitiesAVP(dst []byte, caps uint32) []byte {
	var v [4]byte
	binary.BigEndian.PutUint32(v[:], caps)
	return AppendAVP(dst, true, false, VendorIETF, AVPBearerCapabilities, v[:])
}

func appendFirmwareRevisionAVP(dst []byte, rev uint16) []byte {
	var v [2]byte
	binary.BigEndian.PutUint16(v[:], rev)
	return AppendAVP(dst, false, false, VendorIETF, AVPFirmwareRevision, v[:])
}

func appendHostNameAVP(dst []byte, hostname string) []byte {
	return AppendAVP(dst, true, false, VendorIETF, AVPHostName, []byte(hostname))
}

func appendVendorNameAVP(dst []byte, name string) []byte {
	return AppendAVP(dst, false, false, VendorIETF, AVPVendorName, []byte(name))
}

func appendAssignedTunnelIDAVP(dst []byte, tunnelID uint16) []byte {
	var v [2]byte
	binary.BigEndian.PutUint16(v[:], tunnelID)
	return AppendAVP(dst, true, false, VendorIETF, AVPAssignedTunnelID, v[:])
}

func appendReceiveWindowSizeAVP(dst []byte, size uint16) []byte {
	var v [2]byte
	binary.BigEndian.PutUint16(v[:], size)
	return AppendAVP(dst, true, false, VendorIETF, AVPReceiveWindowSize, v[:])
}

func appendChallengeAVP(dst []byte, challenge []byte) []byte {
	return AppendAVP(dst, true, false, VendorIETF, AVPChallenge, challenge)
}

func appendChallengeResponseAVP(dst []byte, response []byte) []byte {
	return AppendAVP(dst, true, false, VendorIETF, AVPChallengeResponse, response)
}

func appendAssignedSessionIDAVP(dst []byte, sessionID uint16) []byte {
	var v [2]byte
	binary.BigEndian.PutUint16(v[:], sessionID)
	return AppendAVP(dst, true, false, VendorIETF, AVPAssignedSessionID, v[:])
}

func appendCallSerialNumberAVP(dst []byte, serial uint32) []byte {
	var v [4]byte
	binary.BigEndian.PutUint32(v[:], serial)
	return AppendAVP(dst, true, false, VendorIETF, AVPCallSerialNumber, v[:])
}

func appendCallErrorsAVP(dst []byte) []byte {
	// RFC 2661 §4.4.6 / §4.4.28: ICCN MUST include Call Errors.
	// We emit it zero-padded (no errors yet) for spec compliance per
	// spec-finalize finding G2. Body is 2 reserved bytes + 6 u32
	// counters.
	v := make([]byte, 2+6*4)
	return AppendAVP(dst, true, false, VendorIETF, AVPCallErrors, v)
}

func appendACCMAVP(dst []byte) []byte {
	// RFC 1662: ACCM for sync framing is all zeros (no escaping).
	// Spec-finalize G2 calls this out as required in ICCN.
	v := make([]byte, 2+4+4) // reserved(2) + send-accm(4) + recv-accm(4)
	return AppendAVP(dst, true, false, VendorIETF, AVPACCM, v)
}

func appendProxyAuthenTypeAVP(dst []byte, authType uint16) []byte {
	var v [2]byte
	binary.BigEndian.PutUint16(v[:], authType)
	return AppendAVP(dst, false, false, VendorIETF, AVPProxyAuthenType, v[:])
}

func appendProxyAuthenNameAVP(dst []byte, name string) []byte {
	return AppendAVP(dst, false, false, VendorIETF, AVPProxyAuthenName, []byte(name))
}

func appendProxyAuthenChallengeAVP(dst []byte, challenge []byte) []byte {
	return AppendAVP(dst, false, false, VendorIETF, AVPProxyAuthenChallenge, challenge)
}

func appendProxyAuthenResponseAVP(dst []byte, response []byte) []byte {
	return AppendAVP(dst, false, false, VendorIETF, AVPProxyAuthenResponse, response)
}

// Proxy Authen Type AVP values per RFC 2661 §4.4.23.
const (
	ProxyAuthenReserved uint16 = 0
	ProxyAuthenTextual  uint16 = 1
	ProxyAuthenPPPCHAP  uint16 = 2
	ProxyAuthenPPPPAP   uint16 = 3
	ProxyAuthenNoAuth   uint16 = 4
	ProxyAuthenMSCHAPv1 uint16 = 5
)

// DecodeUint16 returns the AVP value interpreted as a u16 in network
// byte order. Caller must have validated `len(a.Value) >= 2`.
func DecodeUint16(a *AVP) uint16 {
	return binary.BigEndian.Uint16(a.Value[:2])
}

// DecodeUint32 returns the AVP value interpreted as a u32 in network
// byte order. Caller must have validated `len(a.Value) >= 4`.
func DecodeUint32(a *AVP) uint32 {
	return binary.BigEndian.Uint32(a.Value[:4])
}

// DecodeString returns the AVP value interpreted as ASCII text.
func DecodeString(a *AVP) string {
	return string(a.Value)
}

// DecodeMessageType returns the Message Type carried by AVP[0] of a
// well-formed control message, or 0 if the first AVP is malformed.
func DecodeMessageType(avps []AVP) uint16 {
	if len(avps) == 0 {
		return 0
	}
	a := &avps[0]
	if a.VendorID != VendorIETF || a.Type != AVPMessageType || len(a.Value) < 2 {
		return 0
	}
	return DecodeUint16(a)
}
