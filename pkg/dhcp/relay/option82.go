// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package relay

import (
	"fmt"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/config/ip"
)

const (
	OptRelayAgentInfo = 82
	SubOptCircuitID   = 1
	SubOptRemoteID    = 2
	SubOptFlags       = 10
	OptEnd            = 255

	FlagUnicast = 0x01
)

type Option82Params struct {
	Interface string
	SVLAN     uint16
	CVLAN     uint16
	MAC       string
}

func FormatOption82Field(format string, p *Option82Params) []byte {
	r := strings.NewReplacer(
		"{interface}", p.Interface,
		"{svlan}", fmt.Sprintf("%d", p.SVLAN),
		"{cvlan}", fmt.Sprintf("%d", p.CVLAN),
		"{mac}", p.MAC,
	)
	return []byte(r.Replace(format))
}

func BuildOption82(cfg *ip.Option82Config, p *Option82Params, unicast bool) ([]byte, error) {
	circuitID := FormatOption82Field(cfg.GetCircuitIDFormat(), p)
	remoteID := FormatOption82Field(cfg.GetRemoteIDFormat(), p)

	if len(circuitID) > 255 {
		return nil, fmt.Errorf("circuit-id exceeds 255 bytes: %d", len(circuitID))
	}
	if len(remoteID) > 255 {
		return nil, fmt.Errorf("remote-id exceeds 255 bytes: %d", len(remoteID))
	}

	totalLen := 2 + len(circuitID) + 2 + len(remoteID)
	if cfg.IncludeFlags {
		totalLen += 3
	}
	if totalLen > 255 {
		return nil, fmt.Errorf("option 82 total length exceeds 255 bytes: %d", totalLen)
	}

	buf := make([]byte, 0, 2+totalLen)
	buf = append(buf, OptRelayAgentInfo, byte(totalLen))

	buf = append(buf, SubOptCircuitID, byte(len(circuitID)))
	buf = append(buf, circuitID...)

	buf = append(buf, SubOptRemoteID, byte(len(remoteID)))
	buf = append(buf, remoteID...)

	if cfg.IncludeFlags {
		flags := byte(0)
		if unicast {
			flags = FlagUnicast
		}
		buf = append(buf, SubOptFlags, 1, flags)
	}

	return buf, nil
}

// InsertOption82 inserts or replaces Option 82 in a raw DHCPv4 packet.
// The packet must include the magic cookie and options section.
// Returns the modified packet.
func InsertOption82(pkt []byte, opt82 []byte, policy string) []byte {
	optStart := 240 // after fixed header (236) + magic cookie (4)
	if len(pkt) < optStart {
		return pkt
	}

	endIdx := -1
	existing82Start := -1
	existing82End := -1

	i := optStart
	for i < len(pkt) {
		if pkt[i] == 0 {
			i++
			continue
		}
		if pkt[i] == OptEnd {
			endIdx = i
			break
		}
		code := pkt[i]
		if i+1 >= len(pkt) {
			break
		}
		optLen := int(pkt[i+1])
		if i+2+optLen > len(pkt) {
			break
		}
		if code == OptRelayAgentInfo {
			existing82Start = i
			existing82End = i + 2 + optLen
		}
		i += 2 + optLen
	}

	if endIdx == -1 {
		endIdx = len(pkt)
	}

	switch policy {
	case "keep":
		if existing82Start >= 0 {
			return pkt
		}
	case "drop":
		if existing82Start >= 0 {
			return removeRange(pkt, existing82Start, existing82End)
		}
		return pkt
	}

	// "replace" (default): remove existing, insert new
	if existing82Start >= 0 {
		pkt = removeRange(pkt, existing82Start, existing82End)
		endIdx -= (existing82End - existing82Start)
	}

	// Insert opt82 before the End option
	result := make([]byte, 0, len(pkt)+len(opt82))
	result = append(result, pkt[:endIdx]...)
	result = append(result, opt82...)
	result = append(result, pkt[endIdx:]...)

	return result
}

// StripOption82 removes Option 82 from a raw DHCPv4 packet.
func StripOption82(pkt []byte) []byte {
	optStart := 240
	if len(pkt) < optStart {
		return pkt
	}

	i := optStart
	for i < len(pkt) {
		if pkt[i] == 0 {
			i++
			continue
		}
		if pkt[i] == OptEnd {
			break
		}
		code := pkt[i]
		if i+1 >= len(pkt) {
			break
		}
		optLen := int(pkt[i+1])
		if i+2+optLen > len(pkt) {
			break
		}
		if code == OptRelayAgentInfo {
			return removeRange(pkt, i, i+2+optLen)
		}
		i += 2 + optLen
	}
	return pkt
}

func removeRange(data []byte, start, end int) []byte {
	result := make([]byte, len(data)-(end-start))
	copy(result, data[:start])
	copy(result[start:], data[end:])
	return result
}
