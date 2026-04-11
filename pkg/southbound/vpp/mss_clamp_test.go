// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"testing"

	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/mss_clamp"
)

func TestDirectionForReturnsBothWhenMSSSet(t *testing.T) {
	if got := directionFor(1460); got != mssClampDirBoth {
		t.Fatalf("directionFor(1460) = %d, want %d (RX|TX)", got, mssClampDirBoth)
	}
}

func TestDirectionForReturnsNoneWhenMSSZero(t *testing.T) {
	if got := directionFor(0); got != mss_clamp.MSS_CLAMP_DIR_NONE {
		t.Fatalf("directionFor(0) = %d, want MSS_CLAMP_DIR_NONE", got)
	}
}

func TestMSSClampDirBothIsBitmaskRXTX(t *testing.T) {
	if uint8(mssClampDirBoth) != uint8(mss_clamp.MSS_CLAMP_DIR_RX)|uint8(mss_clamp.MSS_CLAMP_DIR_TX) {
		t.Fatalf("mssClampDirBoth = %d, want %d", mssClampDirBoth, uint8(mss_clamp.MSS_CLAMP_DIR_RX)|uint8(mss_clamp.MSS_CLAMP_DIR_TX))
	}
	if uint8(mssClampDirBoth) != 3 {
		t.Fatalf("mssClampDirBoth bitmask value = %d, want 3", uint8(mssClampDirBoth))
	}
}
