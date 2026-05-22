// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

import "encoding/json"

type Interface struct {
	Name                       string `json:"-"                            metric:"label=interface,map_key"`
	Status                     string `json:"status,omitempty"             metric:"label"`
	Type                       string `json:"type,omitempty"`
	InterfaceID                int    `json:"interfaceId"                  metric:"name=protocols.ospf6.interface.interface_id,type=gauge,help=OSPFv3 interface index."`
	OperatingAsType            string `json:"operatingAsType,omitempty"`
	AttachedToArea             bool   `json:"attachedToArea,omitempty"     metric:"name=protocols.ospf6.interface.attached_to_area,type=gauge,help=OSPFv3 interface attached to an area (1 if attached)."`
	InstanceID                 int    `json:"instanceId,omitempty"`
	InterfaceMtu               int    `json:"interfaceMtu,omitempty"       metric:"name=protocols.ospf6.interface.mtu_bytes,type=gauge,help=OSPFv3 interface MTU in bytes."`
	AreaID                     string `json:"areaId,omitempty"             metric:"label=area"`
	Cost                       int    `json:"cost,omitempty"               metric:"name=protocols.ospf6.interface.cost,type=gauge,help=OSPFv3 interface output cost (metric)."`
	OSPF6InterfaceState        string `json:"ospf6InterfaceState,omitempty" metric:"label=state"`
	TransmitDelaySec           int    `json:"transmitDelaySec,omitempty"   metric:"name=protocols.ospf6.interface.transmit_delay_seconds,type=gauge,help=OSPFv3 LSA transmit delay in seconds."`
	Priority                   int    `json:"priority,omitempty"           metric:"name=protocols.ospf6.interface.priority,type=gauge,help=OSPFv3 interface DR election priority."`
	TimerIntervalsConfigHello  int    `json:"timerIntervalsConfigHello,omitempty"      metric:"name=protocols.ospf6.interface.hello_interval_seconds,type=gauge,help=OSPFv3 hello interval in seconds."`
	TimerIntervalsConfigDead   int    `json:"timerIntervalsConfigDead,omitempty"       metric:"name=protocols.ospf6.interface.dead_interval_seconds,type=gauge,help=OSPFv3 dead interval in seconds."`
	TimerIntervalsConfigRetx   int    `json:"timerIntervalsConfigRetransmit,omitempty" metric:"name=protocols.ospf6.interface.retransmit_interval_seconds,type=gauge,help=OSPFv3 LSA retransmit interval in seconds."`
	NumberOfInterfaceScopedLsa int    `json:"numberOfInterfaceScopedLsa,omitempty"     metric:"name=protocols.ospf6.interface.scoped_lsa_count,type=gauge,help=OSPFv3 interface-scoped LSA count."`
	PendingLsaLsUpdateCount    int    `json:"pendingLsaLsUpdateCount,omitempty"        metric:"name=protocols.ospf6.interface.pending_lsa_update_count,type=gauge,help=OSPFv3 pending LSA update count on this interface."`
	PendingLsaLsAckCount       int    `json:"pendingLsaLsAckCount,omitempty"           metric:"name=protocols.ospf6.interface.pending_lsa_ack_count,type=gauge,help=OSPFv3 pending LSA ack count on this interface."`
	InternetAddress            []InterfaceAddress `json:"internetAddress,omitempty"`
}

type InterfaceAddress struct {
	Type    string `json:"type,omitempty"`
	Address string `json:"address,omitempty"`
}

type InterfaceTraffic struct {
	Name       string `json:"-"          metric:"label=interface,map_key"`
	HelloRx    int    `json:"helloRx"    metric:"name=protocols.ospf6.interface.hello_rx_total,type=counter,help=Total OSPFv3 hello packets received on this interface."`
	HelloTx    int    `json:"helloTx"    metric:"name=protocols.ospf6.interface.hello_tx_total,type=counter,help=Total OSPFv3 hello packets transmitted on this interface."`
	DbDescRx   int    `json:"dbDescRx"   metric:"name=protocols.ospf6.interface.dbdesc_rx_total,type=counter,help=Total OSPFv3 database description packets received on this interface."`
	DbDescTx   int    `json:"dbDescTx"   metric:"name=protocols.ospf6.interface.dbdesc_tx_total,type=counter,help=Total OSPFv3 database description packets transmitted on this interface."`
	LsReqRx    int    `json:"lsReqRx"    metric:"name=protocols.ospf6.interface.lsreq_rx_total,type=counter,help=Total OSPFv3 LS request packets received on this interface."`
	LsReqTx    int    `json:"lsReqTx"    metric:"name=protocols.ospf6.interface.lsreq_tx_total,type=counter,help=Total OSPFv3 LS request packets transmitted on this interface."`
	LsUpdateRx int    `json:"lsUpdateRx" metric:"name=protocols.ospf6.interface.lsupdate_rx_total,type=counter,help=Total OSPFv3 LS update packets received on this interface."`
	LsUpdateTx int    `json:"lsUpdateTx" metric:"name=protocols.ospf6.interface.lsupdate_tx_total,type=counter,help=Total OSPFv3 LS update packets transmitted on this interface."`
	LsAckRx    int    `json:"lsAckRx"    metric:"name=protocols.ospf6.interface.lsack_rx_total,type=counter,help=Total OSPFv3 LS ack packets received on this interface."`
	LsAckTx    int    `json:"lsAckTx"    metric:"name=protocols.ospf6.interface.lsack_tx_total,type=counter,help=Total OSPFv3 LS ack packets transmitted on this interface."`
}

type InterfacePrefix = json.RawMessage
