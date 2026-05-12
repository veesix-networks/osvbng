// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package routing

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/models/protocols/ldp"
)

func (c *Component) GetLDPNeighbors() ([]ldp.Neighbor, error) {
	output, err := c.execVtysh("-c", "show mpls ldp neighbor json")
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Neighbors []ldp.Neighbor `json:"neighbors"`
	}
	if err := json.Unmarshal(output, &wrapper); err != nil {
		return nil, fmt.Errorf("parse LDP neighbors: %w", err)
	}

	for i := range wrapper.Neighbors {
		wrapper.Neighbors[i].UpTimeSecs = parseLDPUpTime(wrapper.Neighbors[i].UpTime)
	}
	return wrapper.Neighbors, nil
}

func (c *Component) GetLDPBindings() ([]ldp.Binding, error) {
	output, err := c.execVtysh("-c", "show mpls ldp binding json")
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Bindings []ldp.Binding `json:"bindings"`
	}
	if err := json.Unmarshal(output, &wrapper); err != nil {
		return nil, fmt.Errorf("parse LDP bindings: %w", err)
	}
	return wrapper.Bindings, nil
}

func (c *Component) GetLDPDiscovery() ([]ldp.Discovery, error) {
	output, err := c.execVtysh("-c", "show mpls ldp discovery json")
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Adjacencies []ldp.Discovery `json:"adjacencies"`
	}
	if err := json.Unmarshal(output, &wrapper); err != nil {
		return nil, fmt.Errorf("parse LDP discovery: %w", err)
	}
	return wrapper.Adjacencies, nil
}

// parseLDPUpTime converts FRR's human-readable LDP uptime strings
// ("HH:MM:SS", "NdHHhMMm", "NwNd") to seconds. Unparseable input yields
// zero, matching the "absent" semantics for the gauge.
func parseLDPUpTime(s string) uint64 {
	if s == "" {
		return 0
	}

	// HH:MM:SS form.
	if strings.Count(s, ":") == 2 {
		parts := strings.Split(s, ":")
		h, _ := strconv.ParseUint(parts[0], 10, 64)
		m, _ := strconv.ParseUint(parts[1], 10, 64)
		sec, _ := strconv.ParseUint(parts[2], 10, 64)
		return h*3600 + m*60 + sec
	}

	// Compound form: "<num><unit>..." (e.g. "2d3h", "1w2d").
	var total uint64
	var num uint64
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			num = num*10 + uint64(ch-'0')
			continue
		}
		switch ch {
		case 'w':
			total += num * 7 * 24 * 3600
		case 'd':
			total += num * 24 * 3600
		case 'h':
			total += num * 3600
		case 'm':
			total += num * 60
		case 's':
			total += num
		}
		num = 0
	}
	return total
}
