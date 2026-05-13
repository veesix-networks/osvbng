// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"bytes"
	"testing"
)

func TestMessageTypeAVPRoundTrip(t *testing.T) {
	buf := appendMessageTypeAVP(nil, MsgTypeSCCRQ)
	avps, err := ParseAVPs(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := DecodeMessageType(avps); got != MsgTypeSCCRQ {
		t.Fatalf("want %d got %d", MsgTypeSCCRQ, got)
	}
}

func TestResultCodeAVPNoErrorCode(t *testing.T) {
	buf := appendResultCodeAVP(nil, ResultStopGeneralRequest, 0, nil)
	avps, _ := ParseAVPs(buf)
	if len(avps) != 1 || len(avps[0].Value) != 2 {
		t.Fatalf("expected 2-byte value, got %d bytes", len(avps[0].Value))
	}
	if rc := ResultCode(DecodeUint16(&avps[0])); rc != ResultStopGeneralRequest {
		t.Fatalf("want %d got %d", ResultStopGeneralRequest, rc)
	}
}

func TestResultCodeAVPWithErrorCode(t *testing.T) {
	buf := appendResultCodeAVP(nil, ResultStopVersionUnsupported, 0x0100, nil)
	avps, _ := ParseAVPs(buf)
	if len(avps[0].Value) != 4 {
		t.Fatalf("expected 4-byte value, got %d", len(avps[0].Value))
	}
}

func TestAssignedTunnelIDAVP(t *testing.T) {
	buf := appendAssignedTunnelIDAVP(nil, 0xBEEF)
	avps, _ := ParseAVPs(buf)
	if got := DecodeUint16(&avps[0]); got != 0xBEEF {
		t.Fatalf("want 0xBEEF, got 0x%x", got)
	}
	if avps[0].Type != AVPAssignedTunnelID {
		t.Fatalf("type wrong: %d", avps[0].Type)
	}
}

func TestHostNameAVP(t *testing.T) {
	buf := appendHostNameAVP(nil, "bng1.example.net")
	avps, _ := ParseAVPs(buf)
	if got := DecodeString(&avps[0]); got != "bng1.example.net" {
		t.Fatalf("want bng1.example.net, got %q", got)
	}
}

func TestCallErrorsAVPSize(t *testing.T) {
	// RFC 2661 §4.4.28: 2 reserved + 6 u32 counters = 26 body bytes.
	buf := appendCallErrorsAVP(nil)
	avps, _ := ParseAVPs(buf)
	if len(avps[0].Value) != 2+6*4 {
		t.Fatalf("want 26-byte body, got %d", len(avps[0].Value))
	}
}

func TestACCMAVPSize(t *testing.T) {
	// 2 reserved + send accm 4 + recv accm 4 = 10 body bytes.
	buf := appendACCMAVP(nil)
	avps, _ := ParseAVPs(buf)
	if len(avps[0].Value) != 10 {
		t.Fatalf("want 10-byte body, got %d", len(avps[0].Value))
	}
}

func TestAllCatalogConstructorsRoundTrip(t *testing.T) {
	// One AVP from each typed constructor that takes arguments.
	// Builds a full SCCRQ-shaped message body, parses it, and spot-
	// checks the values. Catches off-by-one and byte-order bugs in
	// any of the encoders before the higher-level FSM code starts
	// composing real control messages.
	buf := appendMessageTypeAVP(nil, MsgTypeSCCRQ)
	buf = appendProtocolVersionAVP(buf, 1, 0)
	buf = appendFramingCapabilitiesAVP(buf, FramingSync|FramingAsync)
	buf = appendBearerCapabilitiesAVP(buf, BearerDigital)
	buf = appendFirmwareRevisionAVP(buf, 0x0100)
	buf = appendHostNameAVP(buf, "lac1")
	buf = appendVendorNameAVP(buf, "osvbng")
	buf = appendAssignedTunnelIDAVP(buf, 42)
	buf = appendReceiveWindowSizeAVP(buf, 16)
	buf = appendChallengeAVP(buf, []byte{1, 2, 3, 4, 5, 6, 7, 8})
	buf = appendChallengeResponseAVP(buf, []byte{0xAA, 0xBB, 0xCC, 0xDD})
	buf = appendAssignedSessionIDAVP(buf, 99)
	buf = appendCallSerialNumberAVP(buf, 0xDEADBEEF)
	buf = appendCallErrorsAVP(buf)
	buf = appendACCMAVP(buf)
	buf = appendProxyAuthenTypeAVP(buf, ProxyAuthenPPPCHAP)
	buf = appendProxyAuthenNameAVP(buf, "alice@isp.example")
	buf = appendProxyAuthenChallengeAVP(buf, bytes.Repeat([]byte{0x55}, 16))
	buf = appendProxyAuthenResponseAVP(buf, bytes.Repeat([]byte{0x77}, 16))

	avps, err := ParseAVPs(buf)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := DecodeMessageType(avps); got != MsgTypeSCCRQ {
		t.Fatalf("message type round-trip failed: %d", got)
	}
	if a := FindFirst(avps, 0, AVPAssignedTunnelID); a == nil || DecodeUint16(a) != 42 {
		t.Fatal("AssignedTunnelID round-trip failed")
	}
	if a := FindFirst(avps, 0, AVPReceiveWindowSize); a == nil || DecodeUint16(a) != 16 {
		t.Fatal("ReceiveWindowSize round-trip failed")
	}
	if a := FindFirst(avps, 0, AVPAssignedSessionID); a == nil || DecodeUint16(a) != 99 {
		t.Fatal("AssignedSessionID round-trip failed")
	}
	if a := FindFirst(avps, 0, AVPCallSerialNumber); a == nil || DecodeUint32(a) != 0xDEADBEEF {
		t.Fatal("CallSerialNumber round-trip failed")
	}
	if a := FindFirst(avps, 0, AVPHostName); a == nil || DecodeString(a) != "lac1" {
		t.Fatal("HostName round-trip failed")
	}
	if a := FindFirst(avps, 0, AVPVendorName); a == nil || DecodeString(a) != "osvbng" {
		t.Fatal("VendorName round-trip failed")
	}
	if a := FindFirst(avps, 0, AVPProxyAuthenType); a == nil || DecodeUint16(a) != ProxyAuthenPPPCHAP {
		t.Fatal("ProxyAuthenType round-trip failed")
	}
	if a := FindFirst(avps, 0, AVPProxyAuthenName); a == nil || DecodeString(a) != "alice@isp.example" {
		t.Fatal("ProxyAuthenName round-trip failed")
	}
	if a := FindFirst(avps, 0, AVPChallenge); a == nil || len(a.Value) != 8 {
		t.Fatal("Challenge round-trip failed")
	}
}

func TestMessageTypeName(t *testing.T) {
	cases := map[uint16]string{
		MsgTypeSCCRQ:   "SCCRQ",
		MsgTypeSCCCN:   "SCCCN",
		MsgTypeStopCCN: "StopCCN",
		MsgTypeICCN:    "ICCN",
		MsgTypeCDN:     "CDN",
		99:             "",
	}
	for mt, want := range cases {
		if got := MessageTypeName(mt); got != want {
			t.Errorf("MessageTypeName(%d) = %q, want %q", mt, got, want)
		}
	}
}
