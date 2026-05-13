// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

// Public message builders for the L2TPv2 control messages emitted by
// the osvbng control plane. Each builder returns the AVP-sequence
// body; the L2TP header (with Ns/Nr from the per-tunnel control
// channel) is added by the caller.

// SCCRQParams is the input to BuildSCCRQ. RFC 2661 §6.1 mandatories
// (Message Type, Protocol Version, Framing/Bearer Capabilities, Host
// Name, Assigned Tunnel-ID, Receive Window Size) are always emitted;
// optional fields are skipped when zero-valued.
type SCCRQParams struct {
	LocalTunnelID     uint16
	ReceiveWindowSize uint16
	HostName          string
	VendorName        string
	FramingCaps       uint32
	BearerCaps        uint32
	FirmwareRevision  uint16
	Challenge         []byte
}

// BuildSCCRQ builds a Start-Control-Connection-Request body. Called by
// the LAC to open a new tunnel to an LNS.
func BuildSCCRQ(p SCCRQParams) []byte {
	body := appendMessageTypeAVP(nil, MsgTypeSCCRQ)
	body = appendProtocolVersionAVP(body, 1, 0)
	body = appendFramingCapabilitiesAVP(body, p.FramingCaps)
	body = appendBearerCapabilitiesAVP(body, p.BearerCaps)
	if p.FirmwareRevision != 0 {
		body = appendFirmwareRevisionAVP(body, p.FirmwareRevision)
	}
	body = appendHostNameAVP(body, p.HostName)
	if p.VendorName != "" {
		body = appendVendorNameAVP(body, p.VendorName)
	}
	body = appendAssignedTunnelIDAVP(body, p.LocalTunnelID)
	body = appendReceiveWindowSizeAVP(body, p.ReceiveWindowSize)
	if len(p.Challenge) > 0 {
		body = appendChallengeAVP(body, p.Challenge)
	}
	return body
}

// SCCCNParams is the input to BuildSCCCN. Carries only the (optional)
// Challenge-Response AVP that proves authentication of the LAC against
// the LNS's challenge.
type SCCCNParams struct {
	ChallengeResponse []byte
}

// BuildSCCCNWithParams builds a Start-Control-Connection-Connected
// body. Wraps BuildSCCCN for symmetry with the other Build*Params
// helpers; callers may use either form.
func BuildSCCCNWithParams(p SCCCNParams) []byte {
	return BuildSCCCN(p.ChallengeResponse)
}

// ICRQParams is the input to BuildICRQ.
type ICRQParams struct {
	LocalSessionID   uint16
	CallSerialNumber uint32
}

// BuildICRQ builds an Incoming-Call-Request body. Called by the LAC
// after the tunnel has reached Established to open a new session.
func BuildICRQ(p ICRQParams) []byte {
	body := appendMessageTypeAVP(nil, MsgTypeICRQ)
	body = appendAssignedSessionIDAVP(body, p.LocalSessionID)
	body = appendCallSerialNumberAVP(body, p.CallSerialNumber)
	return body
}

// SCCRPParams is the input to BuildSCCRP. All "M" fields per RFC 2661
// §6.2 are required; optional fields are zero-value to omit.
type SCCRPParams struct {
	LocalTunnelID     uint16
	ReceiveWindowSize uint16
	HostName          string
	VendorName        string
	FramingCaps       uint32
	BearerCaps        uint32
	FirmwareRevision  uint16

	// Challenge and ChallengeResponse are present only when the
	// SCCRQ carried a Challenge AVP and the responder is replying
	// with one of its own + a response per RFC 2661 §5.1.1.
	Challenge         []byte
	ChallengeResponse []byte
}

// BuildSCCRP builds a Start-Control-Connection-Reply body. Called by
// the LNS in response to an SCCRQ from a LAC.
func BuildSCCRP(p SCCRPParams) []byte {
	body := appendMessageTypeAVP(nil, MsgTypeSCCRP)
	body = appendProtocolVersionAVP(body, 1, 0)
	body = appendFramingCapabilitiesAVP(body, p.FramingCaps)
	body = appendBearerCapabilitiesAVP(body, p.BearerCaps)
	if p.FirmwareRevision != 0 {
		body = appendFirmwareRevisionAVP(body, p.FirmwareRevision)
	}
	body = appendHostNameAVP(body, p.HostName)
	if p.VendorName != "" {
		body = appendVendorNameAVP(body, p.VendorName)
	}
	body = appendAssignedTunnelIDAVP(body, p.LocalTunnelID)
	body = appendReceiveWindowSizeAVP(body, p.ReceiveWindowSize)
	if len(p.Challenge) > 0 {
		body = appendChallengeAVP(body, p.Challenge)
	}
	if len(p.ChallengeResponse) > 0 {
		body = appendChallengeResponseAVP(body, p.ChallengeResponse)
	}
	return body
}

// BuildSCCCN builds a Start-Control-Connection-Connected body. Called
// by the LAC after receiving SCCRP.
func BuildSCCCN(challengeResponse []byte) []byte {
	body := appendMessageTypeAVP(nil, MsgTypeSCCCN)
	if len(challengeResponse) > 0 {
		body = appendChallengeResponseAVP(body, challengeResponse)
	}
	return body
}

// BuildStopCCN builds a Stop-Control-Connection-Notification body for
// terminating a control connection.
func BuildStopCCN(localTunnelID uint16, rc ResultCode, ec ErrorCode, msg string) []byte {
	body := appendMessageTypeAVP(nil, MsgTypeStopCCN)
	body = appendResultCodeAVP(body, rc, ec, []byte(msg))
	body = appendAssignedTunnelIDAVP(body, localTunnelID)
	return body
}

// ICRPParams is the input to BuildICRP.
type ICRPParams struct {
	LocalSessionID uint16
}

// BuildICRP builds an Incoming-Call-Reply body. Called by the LNS in
// response to an ICRQ from a LAC.
func BuildICRP(p ICRPParams) []byte {
	body := appendMessageTypeAVP(nil, MsgTypeICRP)
	body = appendAssignedSessionIDAVP(body, p.LocalSessionID)
	return body
}

// ICCNParams is the input to BuildICCN.
type ICCNParams struct {
	TxConnectSpeed uint32 // 0 → omit
	Framing        uint32

	// Optional proxy LCP / proxy auth AVPs from the LAC's PPPoE
	// exchange. Carry-through forms — caller pre-encodes the LCP
	// CONFREQ option strings and the proxy-auth structures.
	LastSentLCPConfReq    []byte
	LastReceivedLCPConfReq []byte
	ProxyAuthenType       uint16
	ProxyAuthenName       string
	ProxyAuthenChallenge  []byte
	ProxyAuthenResponse   []byte
}

// BuildICCN builds an Incoming-Call-Connected body. Called by the LAC
// after receiving ICRP. Carries the proxy-LCP and proxy-auth AVPs that
// pre-state the LNS's PPP termination.
func BuildICCN(p ICCNParams) []byte {
	body := appendMessageTypeAVP(nil, MsgTypeICCN)
	if p.TxConnectSpeed != 0 {
		var v [4]byte
		v[0] = byte(p.TxConnectSpeed >> 24)
		v[1] = byte(p.TxConnectSpeed >> 16)
		v[2] = byte(p.TxConnectSpeed >> 8)
		v[3] = byte(p.TxConnectSpeed)
		body = AppendAVP(body, true, false, VendorIETF, AVPTxConnectSpeed, v[:])
	}
	body = appendFramingCapabilitiesAVP(body, p.Framing)
	// ICCN requires Call Errors and ACCM AVPs per RFC 2661 §4.4.28 /
	// §4.4.29; emit them zero-padded.
	body = appendCallErrorsAVP(body)
	body = appendACCMAVP(body)
	if len(p.LastSentLCPConfReq) > 0 {
		body = AppendAVP(body, true, false, VendorIETF, AVPLastSentLCPConfReq, p.LastSentLCPConfReq)
	}
	if len(p.LastReceivedLCPConfReq) > 0 {
		body = AppendAVP(body, true, false, VendorIETF, AVPLastRecvLCPConfReq, p.LastReceivedLCPConfReq)
	}
	if p.ProxyAuthenType != 0 {
		body = appendProxyAuthenTypeAVP(body, p.ProxyAuthenType)
	}
	if p.ProxyAuthenName != "" {
		body = appendProxyAuthenNameAVP(body, p.ProxyAuthenName)
	}
	if len(p.ProxyAuthenChallenge) > 0 {
		body = appendProxyAuthenChallengeAVP(body, p.ProxyAuthenChallenge)
	}
	if len(p.ProxyAuthenResponse) > 0 {
		body = appendProxyAuthenResponseAVP(body, p.ProxyAuthenResponse)
	}
	return body
}

// BuildCDN builds a Call-Disconnect-Notify body.
func BuildCDN(localSessionID uint16, rc ResultCode, ec ErrorCode, msg string) []byte {
	body := appendMessageTypeAVP(nil, MsgTypeCDN)
	body = appendResultCodeAVP(body, rc, ec, []byte(msg))
	body = appendAssignedSessionIDAVP(body, localSessionID)
	return body
}
