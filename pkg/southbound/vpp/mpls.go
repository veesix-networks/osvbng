package vpp

import (
	"fmt"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/mpls"
	"strings"
)

func (v *VPP) getMPLSTables() ([]*southbound.MPLSTableInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &mpls.MplsTableDump{}
	reqCtx := ch.SendMultiRequest(req)

	var tables []*southbound.MPLSTableInfo
	for {
		reply := &mpls.MplsTableDetails{}
		stop, err := reqCtx.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive MPLS table details: %w", err)
		}

		tables = append(tables, &southbound.MPLSTableInfo{
			TableID: reply.MtTable.MtTableID,
			Name:    strings.TrimRight(reply.MtTable.MtName, "\x00"),
		})
	}

	return tables, nil
}


func (v *VPP) CreateMPLSTable() error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &mpls.MplsTableAddDel{
		MtIsAdd: true,
		MtTable: mpls.MplsTable{
			MtTableID: 0,
			MtName:    "default-mpls",
		},
	}

	reply := &mpls.MplsTableAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("create MPLS table: %w", err)
	}

	v.logger.Info("Created MPLS FIB table", "table_id", 0)
	return nil
}


func (v *VPP) EnableMPLS(swIfIndex uint32) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &mpls.SwInterfaceSetMplsEnable{
		SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
		Enable:    true,
	}

	reply := &mpls.SwInterfaceSetMplsEnableReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("enable MPLS on sw_if_index %d: %w", swIfIndex, err)
	}

	return nil
}


func (v *VPP) GetMPLSRoutes() ([]*southbound.MPLSRouteEntry, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &mpls.MplsRouteDump{
		Table: mpls.MplsTable{MtTableID: 0},
	}
	reqCtx := ch.SendMultiRequest(req)

	var routes []*southbound.MPLSRouteEntry
	for {
		reply := &mpls.MplsRouteDetails{}
		stop, err := reqCtx.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive MPLS route details: %w", err)
		}

		entry := &southbound.MPLSRouteEntry{
			Label:       reply.MrRoute.MrLabel,
			Eos:         reply.MrRoute.MrEos == 1,
			EosProto:    reply.MrRoute.MrEosProto,
			IsMulticast: reply.MrRoute.MrIsMulticast,
		}

		for _, p := range reply.MrRoute.MrPaths {
			path := southbound.MPLSRoutePath{
				SwIfIndex:  p.SwIfIndex,
				Weight:     p.Weight,
				Preference: p.Preference,
			}

			if iface := v.ifMgr.Get(p.SwIfIndex); iface != nil {
				path.Interface = iface.Name
			}

			for i := uint8(0); i < p.NLabels; i++ {
				path.Labels = append(path.Labels, p.LabelStack[i].Label)
			}

			entry.Paths = append(entry.Paths, path)
		}

		routes = append(routes, entry)
	}

	return routes, nil
}


func (v *VPP) GetMPLSInterfaces() ([]*southbound.MPLSInterfaceInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &mpls.MplsInterfaceDump{
		SwIfIndex: interface_types.InterfaceIndex(^uint32(0)),
	}
	reqCtx := ch.SendMultiRequest(req)

	var ifaces []*southbound.MPLSInterfaceInfo
	for {
		reply := &mpls.MplsInterfaceDetails{}
		stop, err := reqCtx.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive MPLS interface details: %w", err)
		}

		info := &southbound.MPLSInterfaceInfo{
			SwIfIndex: uint32(reply.SwIfIndex),
		}
		if iface := v.ifMgr.Get(uint32(reply.SwIfIndex)); iface != nil {
			info.Name = iface.Name
		}

		ifaces = append(ifaces, info)
	}

	return ifaces, nil
}


