// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ip

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	encodingASCII = "ascii"
	encodingHex   = "hex"

	dhcpv4ValueMaxLen = 255
	dhcpv6ValueMaxLen = 65535
)

// DHCPOption is one DHCPv4 (RFC 2132 §2) option entry. On the wire it
// renders as [tag(1)] [length(1)] [value(N)] where length=len(value).
type DHCPOption struct {
	Tag      uint8  `json:"tag"                yaml:"tag"`
	Encoding string `json:"encoding,omitempty" yaml:"encoding,omitempty"`
	Value    string `json:"value"              yaml:"value"`
}

// DHCPv6Option is one DHCPv6 (RFC 8415 §21.1) option entry. On the
// wire it renders as [code(2)] [length(2)] [value(N)] where
// length=len(value).
type DHCPv6Option struct {
	Code     uint16 `json:"code"               yaml:"code"`
	Encoding string `json:"encoding,omitempty" yaml:"encoding,omitempty"`
	Value    string `json:"value"              yaml:"value"`
}

// dhcpv4Denylist holds option tags emitted directly by the pool's
// own fields (subnet mask, router, DNS, lease time, server-id, etc.)
// and tags that belong on the relay (option 82). Allowing them via
// dhcp-options would double-emit on the wire.
var dhcpv4Denylist = map[uint8]string{
	1:   "subnet mask (derived from pool network)",
	3:   "router (set via pool gateway)",
	6:   "DNS servers (set via dns / pool dns)",
	51:  "lease time (set via lease-time)",
	53:  "message type",
	54:  "server identifier",
	82:  "relay agent information (relay-only)",
	121: "classless static routes (derived from address model)",
}

// dhcpv6Denylist holds option codes already emitted by
// pkg/dhcp6.Response.Serialize.
var dhcpv6Denylist = map[uint16]string{
	1:  "OPTION_CLIENTID",
	2:  "OPTION_SERVERID",
	3:  "OPTION_IA_NA (set via iana-pools)",
	5:  "OPTION_IAADDR",
	13: "OPTION_STATUS_CODE",
	23: "OPTION_DNS_SERVERS (set via dns)",
	25: "OPTION_IA_PD (set via pd-pools)",
	26: "OPTION_IAPREFIX",
}

// Decode returns the value bytes for the configured encoding.
func (o DHCPOption) Decode() ([]byte, error) {
	return decodeValue(o.Encoding, o.Value)
}

// Validate enforces tag range, denylist, encoding allowlist, and
// payload length cap.
func (o DHCPOption) Validate() error {
	if o.Tag == 0 || o.Tag == 255 {
		return fmt.Errorf("tag %d is reserved by RFC 2132", o.Tag)
	}
	if reason, banned := dhcpv4Denylist[o.Tag]; banned {
		return fmt.Errorf("tag %d is %s; configure that field instead", o.Tag, reason)
	}
	data, err := o.Decode()
	if err != nil {
		return err
	}
	if len(data) > dhcpv4ValueMaxLen {
		return fmt.Errorf("tag %d: payload %d bytes exceeds DHCPv4 limit of %d", o.Tag, len(data), dhcpv4ValueMaxLen)
	}
	return nil
}

// Decode returns the value bytes for the configured encoding.
func (o DHCPv6Option) Decode() ([]byte, error) {
	return decodeValue(o.Encoding, o.Value)
}

// Validate enforces code range, denylist, encoding allowlist, and
// payload length cap.
func (o DHCPv6Option) Validate() error {
	if o.Code == 0 {
		return errors.New("code 0 is reserved")
	}
	if reason, banned := dhcpv6Denylist[o.Code]; banned {
		return fmt.Errorf("code %d is %s; configure that field instead", o.Code, reason)
	}
	data, err := o.Decode()
	if err != nil {
		return err
	}
	if len(data) > dhcpv6ValueMaxLen {
		return fmt.Errorf("code %d: payload %d bytes exceeds DHCPv6 limit of %d", o.Code, len(data), dhcpv6ValueMaxLen)
	}
	return nil
}

func decodeValue(encoding, value string) ([]byte, error) {
	switch encoding {
	case "", encodingASCII:
		return []byte(value), nil
	case encodingHex:
		cleaned := stripHexSeparators(value)
		if len(cleaned)%2 != 0 {
			return nil, fmt.Errorf("hex value has odd nibble count: %q", value)
		}
		out, err := hex.DecodeString(cleaned)
		if err != nil {
			return nil, fmt.Errorf("hex decode: %w", err)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown encoding %q (want ascii or hex)", encoding)
	}
}

func stripHexSeparators(s string) string {
	r := strings.NewReplacer(":", "", "-", "", " ", "", "\t", "", "\n", "")
	return r.Replace(s)
}
