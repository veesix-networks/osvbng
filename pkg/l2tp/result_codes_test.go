// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import "testing"

func TestDenylistForStopCCN(t *testing.T) {
	cases := map[ResultCode]DenylistKind{
		ResultStopUnauthorized:       DenylistTunnel,
		ResultStopVersionUnsupported: DenylistTunnel,
		ResultStopGeneralError:       DenylistTunnel,
		ResultStopShuttingDown:       DenylistTunnel,
		ResultStopAlreadyExists:      DenylistNone,
	}
	for rc, want := range cases {
		if got := DenylistForStopCCN(rc); got != want {
			t.Errorf("DenylistForStopCCN(%d) = %v, want %v", rc, got, want)
		}
	}
}

func TestDenylistForCDN(t *testing.T) {
	cases := map[ResultCode]DenylistKind{
		ResultCDNTempLackOfFacilities: DenylistTunnel,
		ResultCDNPermLackOfFacilities: DenylistTunnel,
		ResultCDNInvalidDestination:   DenylistTunnel,
		ResultCDNLostCarrier:          DenylistNone,
		ResultCDNTimeout:              DenylistNone,
	}
	for rc, want := range cases {
		if got := DenylistForCDN(rc); got != want {
			t.Errorf("DenylistForCDN(%d) = %v, want %v", rc, got, want)
		}
	}
}

func TestResultCodeStrings(t *testing.T) {
	// Ensure every defined ResultCode emits a non-empty description on
	// the appropriate accessor, and unknown codes emit a clear "unknown"
	// fallback. Avoids accidentally adding a constant without wiring
	// the human-readable name.
	stops := []ResultCode{
		ResultStopGeneralRequest, ResultStopGeneralError,
		ResultStopAlreadyExists, ResultStopUnauthorized,
		ResultStopVersionUnsupported, ResultStopShuttingDown,
		ResultStopFSMError,
	}
	for _, rc := range stops {
		if rc.StopString() == "unknown stopccn result code" {
			t.Errorf("StopString(%d) missing", rc)
		}
	}
	if ResultCode(999).StopString() != "unknown stopccn result code" {
		t.Error("unknown ResultCode should fall through")
	}

	cdns := []ResultCode{
		ResultCDNLostCarrier, ResultCDNGeneralError,
		ResultCDNAdministrative, ResultCDNTempLackOfFacilities,
		ResultCDNPermLackOfFacilities, ResultCDNInvalidDestination,
		ResultCDNNoCarrierDetected, ResultCDNNoDialTone,
		ResultCDNTimeout, ResultCDNNoFramingDetected,
	}
	for _, rc := range cdns {
		if rc.CDNString() == "unknown cdn result code" {
			t.Errorf("CDNString(%d) missing", rc)
		}
	}

	if ErrorNoControlConnection.String() == "unknown error code" {
		t.Error("ErrorNoControlConnection should have name")
	}
	if ErrorCode(99).String() != "unknown error code" {
		t.Error("unknown ErrorCode should fall through")
	}
}
