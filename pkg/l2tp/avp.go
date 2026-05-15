// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// AVP encoder/decoder per RFC 2661 §4.1.
//
// Wire format:
//
//   0                   1                   2                   3
//   0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
//   |M|H|rsvd |       Length        |           Vendor ID           |
//   |         Attribute Type        |        Attribute Value...
//
// Length is a 10-bit field that includes the 6-byte header. Vendor ID
// is 0 for IETF AVPs. Reserved bits must be zero on receive (RFC 2661
// §4.1).

const (
	avpHeaderLen = 6

	avpFlagM   = 1 << 15
	avpFlagH   = 1 << 14
	avpLenMask = 0x03ff
	avpResMask = 0x3c00 // reserved bits — must be zero on receive
)

var (
	ErrAVPShort          = errors.New("l2tp: avp truncated")
	ErrAVPLengthTooBig   = errors.New("l2tp: avp length exceeds buffer")
	ErrAVPLengthTooSmall = errors.New("l2tp: avp length below header size")
	ErrAVPReserved       = errors.New("l2tp: reserved bits set on avp")
	ErrAVPHiddenNoRV     = errors.New("l2tp: hidden avp without random vector seen")
)

// AVP is the parsed view of one Attribute-Value Pair.
type AVP struct {
	Mandatory bool
	Hidden    bool
	VendorID  uint16
	Type      uint16
	Value     []byte // alias into the source buffer; copy if you keep it
}

// ParseAVPs walks a slice of AVPs and returns them in order. The
// returned AVP.Value slices alias into b — callers that need to
// retain data past the buffer's lifetime must copy.
//
// `requireRandomVector` enforces the v1 "no Hidden AVPs without a
// preceding Random Vector AVP" rule: a Hidden AVP encountered before
// a Random Vector AVP yields ErrAVPHiddenNoRV.
func ParseAVPs(b []byte) ([]AVP, error) {
	out := make([]AVP, 0, 8)
	seenRV := false

	for len(b) > 0 {
		if len(b) < avpHeaderLen {
			return nil, ErrAVPShort
		}
		flagsLen := binary.BigEndian.Uint16(b[0:2])
		if flagsLen&avpResMask != 0 {
			return nil, ErrAVPReserved
		}
		length := int(flagsLen & avpLenMask)
		if length < avpHeaderLen {
			return nil, ErrAVPLengthTooSmall
		}
		if length > len(b) {
			return nil, ErrAVPLengthTooBig
		}

		a := AVP{
			Mandatory: (flagsLen & avpFlagM) != 0,
			Hidden:    (flagsLen & avpFlagH) != 0,
			VendorID:  binary.BigEndian.Uint16(b[2:4]),
			Type:      binary.BigEndian.Uint16(b[4:6]),
			Value:     b[avpHeaderLen:length],
		}

		// Random Vector AVP must precede any Hidden AVP per RFC 2661
		// §4.3. We do not implement Hidden decode in v1 but enforce
		// ordering so non-conformant peers are rejected early.
		if a.Hidden && !seenRV {
			return nil, ErrAVPHiddenNoRV
		}
		if a.VendorID == 0 && a.Type == AVPRandomVector {
			seenRV = true
		}

		out = append(out, a)
		b = b[length:]
	}
	return out, nil
}

// FindFirst returns the first AVP matching the given vendor ID and
// type, or nil if not found.
func FindFirst(avps []AVP, vendorID, attrType uint16) *AVP {
	for i := range avps {
		if avps[i].VendorID == vendorID && avps[i].Type == attrType {
			return &avps[i]
		}
	}
	return nil
}

// AppendAVP serializes an AVP into dst and returns the extended slice.
// The Length field is set automatically from len(value)+6.
func AppendAVP(dst []byte, mandatory, hidden bool, vendorID, attrType uint16, value []byte) []byte {
	total := avpHeaderLen + len(value)
	if total > avpLenMask {
		// AVP length is 10 bits, max value carries 1018 bytes.
		// Callers should not pass values this large; we panic so the
		// bug shows up immediately rather than producing corrupt wire
		// data.
		panic(fmt.Sprintf("l2tp: avp value too large (%d bytes)", len(value)))
	}
	flagsLen := uint16(total & avpLenMask)
	if mandatory {
		flagsLen |= avpFlagM
	}
	if hidden {
		flagsLen |= avpFlagH
	}
	dst = binary.BigEndian.AppendUint16(dst, flagsLen)
	dst = binary.BigEndian.AppendUint16(dst, vendorID)
	dst = binary.BigEndian.AppendUint16(dst, attrType)
	dst = append(dst, value...)
	return dst
}

// String renders a single AVP for tracing.
func (a *AVP) String() string {
	return fmt.Sprintf("AVP{v=%d t=%d M=%v H=%v len=%d}",
		a.VendorID, a.Type, a.Mandatory, a.Hidden, len(a.Value))
}
