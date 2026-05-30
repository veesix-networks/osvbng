// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/models"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

// defaultSessionDumpLimit mirrors CGNAT_SESSION_DUMP_DEFAULT_LIMIT in the VPP
// plugin: the page size the plugin applies when the request limit is 0. The
// component needs it to decide HasMore for an unbounded request.
const defaultSessionDumpLimit uint32 = 1024

func protoName(proto uint8) string {
	switch proto {
	case 1:
		return "icmp"
	case 6:
		return "tcp"
	case 17:
		return "udp"
	default:
		return fmt.Sprintf("proto-%d", proto)
	}
}

// DumpSessions returns a page of active CGNAT translations. The plugin filters
// and windows the walk (see CGNATSessionFilter), so this method only maps the
// dataplane rows to models, resolves pool IDs to names, and assembles the page
// envelope. Total is the global live session count (O(1), not filter-scoped).
func (c *Component) DumpSessions(filter models.CGNATSessionFilter) (models.CGNATSessionPage, error) {
	page := models.CGNATSessionPage{Sessions: []models.CGNATSession{}}
	if c == nil || c.dataplane == nil {
		return page, nil
	}

	sessions, err := c.dataplane.CGNATDumpSessions(southbound.CGNATSessionFilter{
		InsideIP:    filter.InsideIP,
		OutsideIP:   filter.OutsideIP,
		RemoteIP:    filter.RemoteIP,
		InsidePort:  filter.InsidePort,
		OutsidePort: filter.OutsidePort,
		RemotePort:  filter.RemotePort,
		Proto:       filter.Proto,
		PoolID:      filter.PoolID,
		StartIndex:  filter.Cursor,
		Limit:       filter.Limit,
	})
	if err != nil {
		return page, fmt.Errorf("dump cgnat sessions: %w", err)
	}

	nameByID := make(map[uint32]string, len(c.poolIDMap))
	for name, id := range c.poolIDMap {
		nameByID[id] = name
	}

	page.Sessions = make([]models.CGNATSession, 0, len(sessions))
	for _, s := range sessions {
		page.Sessions = append(page.Sessions, models.CGNATSession{
			PoolName:       nameByID[s.PoolID],
			PoolID:         s.PoolID,
			InsideIP:       s.InsideIP,
			InsidePort:     s.InsidePort,
			OutsideIP:      s.OutsideIP,
			OutsidePort:    s.OutsidePort,
			RemoteIP:       s.RemoteIP,
			RemotePort:     s.RemotePort,
			Proto:          protoName(s.Proto),
			ALGFlags:       s.ALGFlags,
			Packets:        s.TotalPackets,
			Bytes:          s.TotalBytes,
			AgeSeconds:     s.Age,
			TimeoutSeconds: s.Timeout,
		})
	}

	page.Returned = len(page.Sessions)

	effectiveLimit := filter.Limit
	if effectiveLimit == 0 {
		effectiveLimit = defaultSessionDumpLimit
	}
	if uint32(page.Returned) >= effectiveLimit && page.Returned > 0 {
		page.HasMore = true
		page.NextCursor = sessions[len(sessions)-1].SessionIndex + 1
	}

	if total, err := c.dataplane.CGNATSessionCount(); err != nil {
		c.logger.Warn("cgnat: session count failed; reporting 0", "error", err)
	} else {
		page.Total = total
	}

	return page, nil
}
