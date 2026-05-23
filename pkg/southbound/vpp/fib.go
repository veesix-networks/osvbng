// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/fib_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip_types"
)

func (v *VPP) GetIPv4FIB(tableID uint32) ([]*southbound.IPFIBEntry, error) {
	return v.dumpIPRoutes(tableID, false)
}

func (v *VPP) GetIPv6FIB(tableID uint32) ([]*southbound.IPFIBEntry, error) {
	return v.dumpIPRoutes(tableID, true)
}

func (v *VPP) GetIPv4FIBAll() (map[uint32][]*southbound.IPFIBEntry, error) {
	return v.dumpIPRoutesAll(false)
}

func (v *VPP) GetIPv6FIBAll() (map[uint32][]*southbound.IPFIBEntry, error) {
	return v.dumpIPRoutesAll(true)
}

func (v *VPP) LookupIPv4FIB(tableID uint32, prefix netip.Prefix) (*southbound.IPFIBEntry, error) {
	if !prefix.Addr().Is4() {
		return nil, fmt.Errorf("expected IPv4 prefix, got %q", prefix)
	}
	return v.lookupFIB(tableID, false, prefix)
}

func (v *VPP) LookupIPv6FIB(tableID uint32, prefix netip.Prefix) (*southbound.IPFIBEntry, error) {
	if !prefix.Addr().Is6() || prefix.Addr().Is4In6() {
		return nil, fmt.Errorf("expected IPv6 prefix, got %q", prefix)
	}
	return v.lookupFIB(tableID, true, prefix)
}

func (v *VPP) GetIPv4FIBSummary() (*southbound.IPFIBSummaryAll, error) {
	return v.fibSummaryAll(false)
}

func (v *VPP) GetIPv6FIBSummary() (*southbound.IPFIBSummaryAll, error) {
	return v.fibSummaryAll(true)
}

func (v *VPP) dumpIPRoutes(tableID uint32, isIPv6 bool) ([]*southbound.IPFIBEntry, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &ip.IPRouteDump{
		Table: ip.IPTable{TableID: tableID, IsIP6: isIPv6},
	}
	reqCtx := ch.SendMultiRequest(req)

	var entries []*southbound.IPFIBEntry
	for {
		reply := &ip.IPRouteDetails{}
		stop, err := reqCtx.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive IP route details (table %d, ipv6=%v): %w", tableID, isIPv6, err)
		}
		entries = append(entries, v.ipRouteToEntry(reply.Route))
	}
	return entries, nil
}

func (v *VPP) dumpIPRoutesAll(isIPv6 bool) (map[uint32][]*southbound.IPFIBEntry, error) {
	tables, err := v.GetIPTables()
	if err != nil {
		return nil, fmt.Errorf("list IP tables: %w", err)
	}
	out := make(map[uint32][]*southbound.IPFIBEntry, len(tables))
	for _, t := range tables {
		if t.IsIPv6 != isIPv6 {
			continue
		}
		entries, err := v.dumpIPRoutes(t.TableID, isIPv6)
		if err != nil {
			return nil, err
		}
		out[t.TableID] = entries
	}
	return out, nil
}

func (v *VPP) lookupFIB(tableID uint32, isIPv6 bool, prefix netip.Prefix) (*southbound.IPFIBEntry, error) {
	entries, err := v.dumpIPRoutes(tableID, isIPv6)
	if err != nil {
		return nil, err
	}
	target := prefix.Masked().String()
	for _, e := range entries {
		if e.Prefix == target {
			return e, nil
		}
	}
	return nil, nil
}

func (v *VPP) fibSummaryAll(isIPv6 bool) (*southbound.IPFIBSummaryAll, error) {
	tables, err := v.GetIPTables()
	if err != nil {
		return nil, fmt.Errorf("list IP tables: %w", err)
	}
	out := &southbound.IPFIBSummaryAll{Tables: make(map[uint32]southbound.IPFIBSummary, len(tables))}
	for _, t := range tables {
		if t.IsIPv6 != isIPv6 {
			continue
		}
		entries, err := v.dumpIPRoutes(t.TableID, isIPv6)
		if err != nil {
			return nil, err
		}
		out.Tables[t.TableID] = southbound.IPFIBSummary{
			TableID:    t.TableID,
			Name:       fibTableName(t.TableID, t.Name),
			IsIPv6:     t.IsIPv6,
			EntryCount: uint32(len(entries)),
		}
	}
	return out, nil
}

func (v *VPP) ipRouteToEntry(r ip.IPRoute) *southbound.IPFIBEntry {
	entry := &southbound.IPFIBEntry{
		TableID:    r.TableID,
		Prefix:     r.Prefix.String(),
		StatsIndex: r.StatsIndex,
	}
	if len(r.Paths) == 0 {
		return entry
	}
	entry.Paths = make([]southbound.IPFIBPath, 0, len(r.Paths))
	for _, p := range r.Paths {
		entry.Paths = append(entry.Paths, v.fibPathFromBinapi(p))
	}
	return entry
}

func (v *VPP) fibPathFromBinapi(p fib_types.FibPath) southbound.IPFIBPath {
	path := southbound.IPFIBPath{
		SwIfIndex:  p.SwIfIndex,
		Weight:     p.Weight,
		Preference: p.Preference,
		Type:       fibPathTypeName(p.Type),
		Proto:      fibPathProtoName(p.Proto),
	}
	if iface := v.ifMgr.Get(p.SwIfIndex); iface != nil {
		path.Interface = iface.Name
	}
	if nh := fibNextHopString(p.Proto, p.Nh.Address); nh != "" {
		path.NextHop = nh
	}
	for i := uint8(0); i < p.NLabels && int(i) < len(p.LabelStack); i++ {
		path.Labels = append(path.Labels, p.LabelStack[i].Label)
	}
	return path
}

func fibTableName(id uint32, name string) string {
	name = strings.TrimRight(name, "\x00")
	if name != "" {
		return name
	}
	if id == 0 {
		return "default"
	}
	return ""
}

func fibPathTypeName(t fib_types.FibPathType) string {
	switch t {
	case fib_types.FIB_API_PATH_TYPE_NORMAL:
		return "normal"
	case fib_types.FIB_API_PATH_TYPE_LOCAL:
		return "local"
	case fib_types.FIB_API_PATH_TYPE_DROP:
		return "drop"
	case fib_types.FIB_API_PATH_TYPE_UDP_ENCAP:
		return "udp-encap"
	case fib_types.FIB_API_PATH_TYPE_BIER_IMP:
		return "bier-imp"
	case fib_types.FIB_API_PATH_TYPE_ICMP_UNREACH:
		return "icmp-unreach"
	case fib_types.FIB_API_PATH_TYPE_ICMP_PROHIBIT:
		return "icmp-prohibit"
	case fib_types.FIB_API_PATH_TYPE_SOURCE_LOOKUP:
		return "source-lookup"
	case fib_types.FIB_API_PATH_TYPE_DVR:
		return "dvr"
	case fib_types.FIB_API_PATH_TYPE_INTERFACE_RX:
		return "interface-rx"
	case fib_types.FIB_API_PATH_TYPE_CLASSIFY:
		return "classify"
	default:
		return "unknown"
	}
}

func fibPathProtoName(p fib_types.FibPathNhProto) string {
	switch p {
	case fib_types.FIB_API_PATH_NH_PROTO_IP4:
		return "ip4"
	case fib_types.FIB_API_PATH_NH_PROTO_IP6:
		return "ip6"
	case fib_types.FIB_API_PATH_NH_PROTO_MPLS:
		return "mpls"
	case fib_types.FIB_API_PATH_NH_PROTO_ETHERNET:
		return "ethernet"
	case fib_types.FIB_API_PATH_NH_PROTO_BIER:
		return "bier"
	default:
		return "unknown"
	}
}

func fibNextHopString(proto fib_types.FibPathNhProto, un ip_types.AddressUnion) string {
	switch proto {
	case fib_types.FIB_API_PATH_NH_PROTO_IP4:
		addr := un.GetIP4()
		if addr == (ip_types.IP4Address{}) {
			return ""
		}
		ip, _ := netip.AddrFromSlice(addr[:])
		return ip.String()
	case fib_types.FIB_API_PATH_NH_PROTO_IP6:
		addr := un.GetIP6()
		if addr == (ip_types.IP6Address{}) {
			return ""
		}
		ip, _ := netip.AddrFromSlice(addr[:])
		return ip.String()
	default:
		return ""
	}
}
