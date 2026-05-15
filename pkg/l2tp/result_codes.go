// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

// StopCCN and CDN Result Codes per RFC 2661 §4.4.2. Result Codes are
// scoped to the message that carries them: StopCCN values 1..7 are
// valid only in StopCCN, CDN values 1..11 only in CDN.

type ResultCode uint16

const (
	ResultStopReserved ResultCode = 0

	ResultStopGeneralRequest     ResultCode = 1
	ResultStopGeneralError       ResultCode = 2
	ResultStopAlreadyExists      ResultCode = 3
	ResultStopUnauthorized       ResultCode = 4
	ResultStopVersionUnsupported ResultCode = 5
	ResultStopShuttingDown       ResultCode = 6
	ResultStopFSMError           ResultCode = 7

	ResultCDNLostCarrier          ResultCode = 1
	ResultCDNGeneralError         ResultCode = 2
	ResultCDNAdministrative       ResultCode = 3
	ResultCDNTempLackOfFacilities ResultCode = 4
	ResultCDNPermLackOfFacilities ResultCode = 5
	ResultCDNInvalidDestination   ResultCode = 6
	ResultCDNNoCarrierDetected    ResultCode = 7
	ResultCDNNoDialTone           ResultCode = 8
	ResultCDNTimeout              ResultCode = 9
	ResultCDNNoFramingDetected    ResultCode = 10
)

// Error Code AVP values per RFC 2661 §4.4.2. Carried alongside the
// Result Code in StopCCN / CDN when the Result Code is "General error".

type ErrorCode uint16

const (
	ErrorNoGeneralError        ErrorCode = 0
	ErrorNoControlConnection   ErrorCode = 1
	ErrorLengthWrong           ErrorCode = 2
	ErrorOutOfRange            ErrorCode = 3
	ErrorInsufficientResources ErrorCode = 4
	ErrorInvalidSessionID      ErrorCode = 5
	ErrorVendorSpecific        ErrorCode = 6
	ErrorTryAnotherLNS         ErrorCode = 7
	ErrorUnknownMandatoryAVP   ErrorCode = 8
)

func (r ResultCode) StopString() string {
	switch r {
	case ResultStopGeneralRequest:
		return "general request to clear control connection"
	case ResultStopGeneralError:
		return "general error (see error code)"
	case ResultStopAlreadyExists:
		return "control channel already exists"
	case ResultStopUnauthorized:
		return "requester not authorized"
	case ResultStopVersionUnsupported:
		return "protocol version not supported"
	case ResultStopShuttingDown:
		return "requester is being shut down"
	case ResultStopFSMError:
		return "FSM error"
	}
	return "unknown stopccn result code"
}

func (r ResultCode) CDNString() string {
	switch r {
	case ResultCDNLostCarrier:
		return "call lost carrier"
	case ResultCDNGeneralError:
		return "call disconnected (see error code)"
	case ResultCDNAdministrative:
		return "call disconnected administratively"
	case ResultCDNTempLackOfFacilities:
		return "temporary lack of facilities"
	case ResultCDNPermLackOfFacilities:
		return "permanent lack of facilities"
	case ResultCDNInvalidDestination:
		return "invalid destination"
	case ResultCDNNoCarrierDetected:
		return "no carrier detected"
	case ResultCDNNoDialTone:
		return "no dial tone"
	case ResultCDNTimeout:
		return "call not established within time"
	case ResultCDNNoFramingDetected:
		return "no framing detected"
	}
	return "unknown cdn result code"
}

func (e ErrorCode) String() string {
	switch e {
	case ErrorNoGeneralError:
		return "no general error"
	case ErrorNoControlConnection:
		return "no control connection for this LAC-LNS pair"
	case ErrorLengthWrong:
		return "length wrong"
	case ErrorOutOfRange:
		return "field value out of range or reserved"
	case ErrorInsufficientResources:
		return "insufficient resources"
	case ErrorInvalidSessionID:
		return "invalid session id in this context"
	case ErrorVendorSpecific:
		return "vendor-specific error"
	case ErrorTryAnotherLNS:
		return "try another LNS"
	case ErrorUnknownMandatoryAVP:
		return "unknown mandatory AVP"
	}
	return "unknown error code"
}

// DenylistKind classifies what to denylist on a StopCCN/CDN result.
// Used by the LAC's multi-LNS preference selector (RND.md §11).
type DenylistKind int

const (
	DenylistNone DenylistKind = iota
	DenylistTunnel
	DenylistPeer
)

// DenylistForStopCCN returns the appropriate denylist scope for a
// StopCCN Result Code per the policy in RND.md §11 (refined by C6).
func DenylistForStopCCN(rc ResultCode) DenylistKind {
	switch rc {
	case ResultStopUnauthorized, ResultStopVersionUnsupported:
		return DenylistTunnel
	case ResultStopGeneralError, ResultStopGeneralRequest, ResultStopShuttingDown:
		return DenylistTunnel
	}
	return DenylistNone
}

// DenylistForCDN returns the appropriate denylist scope for a CDN
// Result Code during session setup.
func DenylistForCDN(rc ResultCode) DenylistKind {
	switch rc {
	case ResultCDNTempLackOfFacilities,
		ResultCDNPermLackOfFacilities,
		ResultCDNInvalidDestination:
		return DenylistTunnel
	}
	return DenylistNone
}
