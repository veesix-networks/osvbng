package vpp

import (
	"fmt"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/ip"
	"strings"
)

func (v *VPP) GetIPTables() ([]*southbound.IPTableInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &ip.IPTableDump{}
	reqCtx := ch.SendMultiRequest(req)

	var tables []*southbound.IPTableInfo
	for {
		reply := &ip.IPTableDetails{}
		stop, err := reqCtx.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive IP table details: %w", err)
		}

		tables = append(tables, &southbound.IPTableInfo{
			TableID: reply.Table.TableID,
			Name:    strings.TrimRight(reply.Table.Name, "\x00"),
			IsIPv6:  reply.Table.IsIP6,
		})
	}

	return tables, nil
}


func (v *VPP) GetIPMTables() ([]*southbound.IPMTableInfo, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return nil, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &ip.IPMtableDump{}
	reqCtx := ch.SendMultiRequest(req)

	var tables []*southbound.IPMTableInfo
	for {
		reply := &ip.IPMtableDetails{}
		stop, err := reqCtx.ReceiveReply(reply)
		if stop {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("receive IP mtable details: %w", err)
		}

		tables = append(tables, &southbound.IPMTableInfo{
			TableID: reply.Table.TableID,
			Name:    strings.TrimRight(reply.Table.Name, "\x00"),
			IsIPv6:  reply.Table.IsIP6,
		})
	}

	return tables, nil
}


func (v *VPP) GetNextAvailableGlobalTableId() (uint32, error) {
	usedIDs := make(map[uint32]bool)

	usedIDs[0] = true

	ipTables, _ := v.GetIPTables()
	for _, t := range ipTables {
		usedIDs[t.TableID] = true
	}

	mTables, _ := v.GetIPMTables()
	for _, t := range mTables {
		usedIDs[t.TableID] = true
	}

	mplsTables, _ := v.getMPLSTables()
	for _, t := range mplsTables {
		usedIDs[t.TableID] = true
	}

	for i := uint32(1); i < 4294967295; i++ {
		if !usedIDs[i] {
			return i, nil
		}
	}

	// Is this risky to return 0? 0 is the default... but we do return an error, but someone could just ignore the error
	return 0, fmt.Errorf("no available table IDs")
}


func (v *VPP) AddIPTable(tableID uint32, isIPv6 bool, name string) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &ip.IPTableAddDel{
		IsAdd: true,
		Table: ip.IPTable{
			TableID: tableID,
			IsIP6:   isIPv6,
			Name:    name,
		},
	}

	reply := &ip.IPTableAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("add IP table %d (ipv6=%v): %w", tableID, isIPv6, err)
	}

	return nil
}


func (v *VPP) DelIPTable(tableID uint32, isIPv6 bool) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &ip.IPTableAddDel{
		IsAdd: false,
		Table: ip.IPTable{
			TableID: tableID,
			IsIP6:   isIPv6,
		},
	}

	reply := &ip.IPTableAddDelReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return fmt.Errorf("delete IP table %d (ipv6=%v): %w", tableID, isIPv6, err)
	}

	return nil
}


