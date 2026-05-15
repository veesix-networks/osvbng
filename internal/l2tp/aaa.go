// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/aaa"
)

// TunnelSpec is one candidate LNS tunnel derived from a single tagged
// set of RFC 2868 attributes (Tunnel-Type:tag, Tunnel-Server-Endpoint:tag,
// Tunnel-Password:tag, …) returned by AAA. The LAC builds an ordered
// list of these and tries them in Preference order.
type TunnelSpec struct {
	Tag           uint8
	ServerIP      net.IP
	ClientIP      net.IP
	Password      string
	AssignmentID  string
	Preference    uint16
	ClientAuthID  string
	ServerAuthID  string

	// PPP header skip on data frames (0 or 2). Resolved from the
	// LAC's tunnel-pool entry + profile config. Defaults to 2 (HDLC
	// Address+Control prefix present) to match every major LNS out
	// of the box.
	PPPHdrSkip uint8
}

// ParseTunnelSpecs reads an AAA attribute map and returns the tunnel
// candidates in preference order. Tagged attributes use the form
// `<base>:<tag>` (e.g. "tunnel.server-endpoint:1"). Untagged values
// land in the tag-0 group, which RFC 2868 §3.4 defines as the
// "default" tag.
func ParseTunnelSpecs(attrs map[string]string) []TunnelSpec {
	if len(attrs) == 0 {
		return nil
	}

	byTag := map[uint8]*TunnelSpec{}

	get := func(k string) *TunnelSpec {
		base, tag := splitTag(k)
		_ = base
		s, ok := byTag[tag]
		if !ok {
			s = &TunnelSpec{Tag: tag, PPPHdrSkip: 2}
			byTag[tag] = s
		}
		return s
	}

	for k, v := range attrs {
		base, _ := splitTag(k)
		switch base {
		case aaa.AttrTunnelType:
			if !strings.EqualFold(v, "L2TP") {
				continue
			}
		case aaa.AttrTunnelMediumType:
			if !strings.EqualFold(v, "IPv4") {
				continue
			}
		case aaa.AttrTunnelServerEndpoint:
			get(k).ServerIP = net.ParseIP(v)
		case aaa.AttrTunnelClientEndpoint:
			get(k).ClientIP = net.ParseIP(v)
		case aaa.AttrTunnelPassword:
			get(k).Password = v
		case aaa.AttrTunnelAssignmentID:
			get(k).AssignmentID = v
		case aaa.AttrTunnelPreference:
			if p, err := strconv.ParseUint(v, 10, 16); err == nil {
				get(k).Preference = uint16(p)
			}
		case aaa.AttrTunnelClientAuthID:
			get(k).ClientAuthID = v
		case aaa.AttrTunnelServerAuthID:
			get(k).ServerAuthID = v
		}
	}

	out := make([]TunnelSpec, 0, len(byTag))
	for _, s := range byTag {
		if s.ServerIP == nil {
			continue
		}
		out = append(out, *s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Preference < out[j].Preference
	})
	return out
}

// splitTag extracts the base attribute name and tag value from a
// possibly-tagged key. "tunnel.server-endpoint:5" → ("tunnel.server-endpoint", 5).
// Unrecognised tag values default to 0.
func splitTag(k string) (string, uint8) {
	idx := strings.LastIndexByte(k, ':')
	if idx == -1 {
		return k, 0
	}
	t, err := strconv.ParseUint(k[idx+1:], 10, 8)
	if err != nil {
		return k, 0
	}
	return k[:idx], uint8(t)
}
