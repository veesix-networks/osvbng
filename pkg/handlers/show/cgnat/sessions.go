// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/models"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &SessionsHandler{deps: d}
	})
}

type SessionsHandler struct {
	deps *deps.ShowDeps
}

// SessionFilterOptions are the query parameters for the session dump. cursor and
// limit drive backend windowing (the plugin filters and pages the table), so
// this handler returns the page envelope directly and is not subject to the
// generic presentation-layer pagination.
type SessionFilterOptions struct {
	InsideIP    string `query:"inside-ip" description:"Filter on subscriber address." format:"ip-address"`
	OutsideIP   string `query:"outside-ip" description:"Filter on translated address." format:"ip-address"`
	RemoteIP    string `query:"remote-ip" description:"Filter on remote peer address." format:"ip-address"`
	InsidePort  uint16 `query:"inside-port" description:"Filter on subscriber port; the ICMP identifier for ICMP."`
	OutsidePort uint16 `query:"outside-port" description:"Filter on translated port; the translated ICMP identifier for ICMP."`
	RemotePort  uint16 `query:"remote-port" description:"Filter on remote port; not valid with proto=icmp (remote port is 0 for ICMP)."`
	Proto       string `query:"proto" description:"Filter on protocol." enum:"tcp,udp,icmp"`
	PoolID      uint32 `query:"pool-id" description:"Filter on pool id."`
	Cursor      uint32 `query:"cursor" description:"Resume cursor from a previous page's next_cursor (0 = from the start)."`
	Limit       uint32 `query:"limit" description:"Max sessions per page (0 = plugin default)."`
}

func (h *SessionsHandler) Collect(_ context.Context, req *show.Request) (interface{}, error) {
	if h.deps.CGNAT == nil {
		return models.CGNATSessionPage{Sessions: []models.CGNATSession{}}, nil
	}

	filter, err := parseSessionFilter(req)
	if err != nil {
		return nil, err
	}

	page, err := h.deps.CGNAT.DumpSessions(filter)
	if err != nil {
		return nil, err
	}
	return page, nil
}

func parseSessionFilter(req *show.Request) (models.CGNATSessionFilter, error) {
	var f models.CGNATSessionFilter

	for opt, dst := range map[string]*net.IP{
		"inside-ip":  &f.InsideIP,
		"outside-ip": &f.OutsideIP,
		"remote-ip":  &f.RemoteIP,
	} {
		if v := req.Options[opt]; v != "" {
			ip := net.ParseIP(v)
			if ip == nil || ip.To4() == nil {
				return f, fmt.Errorf("invalid %s: %q (expected an IPv4 address)", opt, v)
			}
			*dst = ip.To4()
		}
	}

	for opt, dst := range map[string]*uint16{
		"inside-port":  &f.InsidePort,
		"outside-port": &f.OutsidePort,
		"remote-port":  &f.RemotePort,
	} {
		if v := req.Options[opt]; v != "" {
			p, err := strconv.ParseUint(v, 10, 16)
			if err != nil {
				return f, fmt.Errorf("invalid %s: %q", opt, v)
			}
			*dst = uint16(p)
		}
	}

	for opt, dst := range map[string]*uint32{
		"pool-id": &f.PoolID,
		"cursor":  &f.Cursor,
		"limit":   &f.Limit,
	} {
		if v := req.Options[opt]; v != "" {
			n, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return f, fmt.Errorf("invalid %s: %q", opt, v)
			}
			*dst = uint32(n)
		}
	}

	switch v := req.Options["proto"]; v {
	case "":
	case "icmp":
		f.Proto = 1
	case "tcp":
		f.Proto = 6
	case "udp":
		f.Proto = 17
	default:
		return f, fmt.Errorf("invalid proto: %q (expected tcp, udp, or icmp)", v)
	}

	if f.Proto == 1 && f.RemotePort != 0 {
		return f, fmt.Errorf("remote-port filter is not valid with proto=icmp: ICMP carries no remote port (it is 0); the ICMP identifier is in inside-port/outside-port")
	}

	return f, nil
}

func (h *SessionsHandler) PathPattern() paths.Path {
	return paths.CGNATSessions
}

func (h *SessionsHandler) Dependencies() []paths.Path {
	return nil
}

func (h *SessionsHandler) OptionsType() interface{} {
	return &SessionFilterOptions{}
}

func (h *SessionsHandler) OutputType() interface{} {
	return models.CGNATSessionPage{}
}

func (h *SessionsHandler) Summary() string {
	return "List active CGNAT sessions (translations)"
}

func (h *SessionsHandler) Description() string {
	return "Dump active CGNAT NAT translations filtered by inside/outside/remote address, port, protocol, and pool. The plugin filters and windows the table; page with cursor/limit and follow next_cursor until has_more is false. total is the global live session count."
}
