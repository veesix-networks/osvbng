// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// L2TPv2 header per RFC 2661 §3.1. The first 16 bits carry flag bits
// and the version field. Subsequent fields appear conditionally based
// on the flags:
//
//   bit 0 (T): Type — 1 control, 0 data
//   bit 1 (L): Length field present
//   bits 2-3:  reserved (must be 0)
//   bit 4 (S): Ns / Nr present
//   bit 5:     reserved
//   bit 6 (O): Offset Size present
//   bit 7 (P): Priority (data only)
//   bits 8-11: reserved
//   bits 12-15: Version (must be 2)
//
//   Tunnel ID (16 bits)
//   Session ID (16 bits)
//   [ Ns (16 bits)  ] if S=1
//   [ Nr (16 bits)  ] if S=1
//   [ Offset Size N + N pad bytes ] if O=1
//
// Length field, when present, appears between the flags/version word
// and the Tunnel ID word. The length covers the entire L2TP message
// including header and payload.

const (
	flagT = 1 << 15
	flagL = 1 << 14
	flagS = 1 << 11
	flagO = 1 << 9
	flagP = 1 << 8

	verMask = 0x000f
	// Reserved bits per RFC 2661 §3.1 — must be zero on receive.
	// In network-byte-order u16 these are:
	//   bits 2-3 (RFC) → u16 bits 13,12 → 0x3000
	//   bit 5  (RFC)   → u16 bit 10     → 0x0400
	//   bits 8-11 (RFC) → u16 bits 7..4  → 0x00f0
	resMask = 0x34f0
)

// Version is the protocol version field. Only Version 2 is parsed
// fully; Version 3 (L2TPv3, RFC 3931) is detected and rejected
// elsewhere via v3_detect.go.
const (
	Version2 uint8 = 2
	Version3 uint8 = 3
)

var (
	ErrShortPacket   = errors.New("l2tp: short packet")
	ErrBadVersion    = errors.New("l2tp: bad version")
	ErrReservedBits  = errors.New("l2tp: reserved bits set in flags")
	ErrLengthInvalid = errors.New("l2tp: length field smaller than header")
)

// Header is the parsed view of an L2TPv2 header. Field presence flags
// are kept so the build path can round-trip identically. Ns/Nr/Offset
// fields are zero when their flag is unset.
type Header struct {
	IsControl   bool
	HasLength   bool
	HasSequence bool
	HasOffset   bool
	Priority    bool

	Version uint8

	Length    uint16
	TunnelID  uint16
	SessionID uint16
	Ns        uint16
	Nr        uint16

	// OffsetSize is the number of pad bytes between the L2TP header
	// and the payload, plus 2 bytes for the OffsetSize field itself.
	// Only meaningful when HasOffset is true.
	OffsetSize uint16

	// HeaderLen is the number of bytes consumed by the header.
	HeaderLen int
}

// MinHeaderLen is the smallest possible L2TPv2 header — no length, no
// sequence, no offset.
const MinHeaderLen = 6

// Parse reads an L2TPv2 header from b. Returns the parsed header and
// the slice positioned at the payload (after the header and any
// offset padding). Does not validate Version against Version2; v3
// detection is the caller's job.
func Parse(b []byte) (*Header, []byte, error) {
	if len(b) < MinHeaderLen {
		return nil, nil, ErrShortPacket
	}

	flags := binary.BigEndian.Uint16(b[0:2])
	if flags&resMask != 0 {
		return nil, nil, ErrReservedBits
	}

	h := &Header{
		IsControl:   (flags & flagT) != 0,
		HasLength:   (flags & flagL) != 0,
		HasSequence: (flags & flagS) != 0,
		HasOffset:   (flags & flagO) != 0,
		Priority:    (flags & flagP) != 0,
		Version:     uint8(flags & verMask),
	}

	off := 2
	if h.HasLength {
		if len(b) < off+2 {
			return nil, nil, ErrShortPacket
		}
		h.Length = binary.BigEndian.Uint16(b[off : off+2])
		off += 2
	}

	if len(b) < off+4 {
		return nil, nil, ErrShortPacket
	}
	h.TunnelID = binary.BigEndian.Uint16(b[off : off+2])
	h.SessionID = binary.BigEndian.Uint16(b[off+2 : off+4])
	off += 4

	if h.HasSequence {
		if len(b) < off+4 {
			return nil, nil, ErrShortPacket
		}
		h.Ns = binary.BigEndian.Uint16(b[off : off+2])
		h.Nr = binary.BigEndian.Uint16(b[off+2 : off+4])
		off += 4
	}

	if h.HasOffset {
		if len(b) < off+2 {
			return nil, nil, ErrShortPacket
		}
		h.OffsetSize = binary.BigEndian.Uint16(b[off : off+2])
		off += 2
		// OffsetSize is the count of pad bytes following the
		// OffsetSize field itself.
		if len(b) < off+int(h.OffsetSize) {
			return nil, nil, ErrShortPacket
		}
		off += int(h.OffsetSize)
	}

	if h.HasLength {
		if int(h.Length) < off {
			return nil, nil, ErrLengthInvalid
		}
		if len(b) < int(h.Length) {
			return nil, nil, ErrShortPacket
		}
	}

	h.HeaderLen = off
	return h, b[off:], nil
}

// HeaderLen returns the number of bytes AppendTo writes for this
// header configuration. Stable for a given (HasLength, HasSequence,
// HasOffset, OffsetSize) combination.
func (h *Header) HeaderLenBytes() int {
	n := 2 + 4 // flags + tunnel + session
	if h.HasLength {
		n += 2
	}
	if h.HasSequence {
		n += 4
	}
	if h.HasOffset {
		n += 2 + int(h.OffsetSize)
	}
	return n
}

// AppendTo serializes the header into dst. The returned slice is dst
// extended by the header bytes. When HasLength is set, the L field
// is filled with `HeaderLenBytes() + bodyLen` so the caller does not
// have to patch it after appending the body.
func (h *Header) AppendTo(dst []byte, bodyLen int) []byte {
	var flags uint16
	if h.IsControl {
		flags |= flagT
	}
	if h.HasLength {
		flags |= flagL
	}
	if h.HasSequence {
		flags |= flagS
	}
	if h.HasOffset {
		flags |= flagO
	}
	if h.Priority {
		flags |= flagP
	}
	flags |= uint16(h.Version) & verMask

	dst = binary.BigEndian.AppendUint16(dst, flags)

	if h.HasLength {
		total := uint16(h.HeaderLenBytes() + bodyLen)
		dst = binary.BigEndian.AppendUint16(dst, total)
	}

	dst = binary.BigEndian.AppendUint16(dst, h.TunnelID)
	dst = binary.BigEndian.AppendUint16(dst, h.SessionID)

	if h.HasSequence {
		dst = binary.BigEndian.AppendUint16(dst, h.Ns)
		dst = binary.BigEndian.AppendUint16(dst, h.Nr)
	}

	if h.HasOffset {
		dst = binary.BigEndian.AppendUint16(dst, h.OffsetSize)
		for i := 0; i < int(h.OffsetSize); i++ {
			dst = append(dst, 0)
		}
	}

	return dst
}

// String renders a compact summary, useful for logs and tracing.
func (h *Header) String() string {
	kind := "data"
	if h.IsControl {
		kind = "ctrl"
	}
	s := fmt.Sprintf("L2TPv%d %s tid=%d sid=%d", h.Version, kind, h.TunnelID, h.SessionID)
	if h.HasSequence {
		s += fmt.Sprintf(" Ns=%d Nr=%d", h.Ns, h.Nr)
	}
	if h.HasLength {
		s += fmt.Sprintf(" len=%d", h.Length)
	}
	return s
}

// NewControl returns a control-message header pre-populated with the
// fields every control message needs: T=1, L=1, S=1, version=2.
func NewControl(tunnelID, sessionID, ns, nr uint16) *Header {
	return &Header{
		IsControl:   true,
		HasLength:   true,
		HasSequence: true,
		Version:     Version2,
		TunnelID:    tunnelID,
		SessionID:   sessionID,
		Ns:          ns,
		Nr:          nr,
	}
}

// NewData returns a data-message header. Sequence numbers are
// optional on data messages (default off, per RFC 2661 §3.1).
func NewData(tunnelID, sessionID uint16) *Header {
	return &Header{
		IsControl: false,
		Version:   Version2,
		TunnelID:  tunnelID,
		SessionID: sessionID,
	}
}
