// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package interfaces

import (
	"context"
	"fmt"
	"net"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	show.RegisterFactory(NewDetailHandler)
}

type DetailHandler struct {
	southbound southbound.Southbound
}

func NewDetailHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &DetailHandler{southbound: deps.Southbound}
}

func (h *DetailHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	wildcards, err := paths.Extract(req.Path, paths.InterfacesDetail)
	if err != nil {
		return nil, fmt.Errorf("extract interface name: %w", err)
	}
	if len(wildcards) != 1 {
		return nil, fmt.Errorf("invalid path: expected 1 wildcard, got %d", len(wildcards))
	}

	name := wildcards[0]
	iface := h.southbound.GetIfMgr().GetByName(name)
	if iface == nil {
		return nil, fmt.Errorf("interface not found: %s", name)
	}

	detail := InterfaceDetail{
		Name:        iface.Name,
		SwIfIndex:   iface.SwIfIndex,
		AdminUp:     iface.AdminUp,
		LinkUp:      iface.LinkUp,
		MTU:         iface.MTU,
		MAC:         net.HardwareAddr(iface.MAC).String(),
		LinkSpeed:   iface.LinkSpeed,
		Description: iface.Tag,
		FIBTableID:  iface.FIBTableID,
	}

	for _, ip := range iface.IPv4Addresses {
		detail.IPv4Addresses = append(detail.IPv4Addresses, ip.String())
	}
	for _, ip := range iface.IPv6Addresses {
		detail.IPv6Addresses = append(detail.IPv6Addresses, ip.String())
	}

	if stats, err := h.southbound.GetInterfaceStats(); err == nil {
		for _, s := range stats {
			if s.Index == iface.SwIfIndex {
				detail.Stats = &InterfaceDetailStats{
					RxPackets: s.Rx,
					RxBytes:   s.RxBytes,
					RxErrors:  s.RxErrors,
					TxPackets: s.Tx,
					TxBytes:   s.TxBytes,
					TxErrors:  s.TxErrors,
					Drops:     s.Drops,
				}
				break
			}
		}
	}

	if iface.DevType == "bond" {
		h.collectBondSection(&detail, iface.SwIfIndex)
	}

	if iface.Type == ifmgr.IfTypeSub {
		h.collectSubinterfaceSection(&detail, iface)
	}

	return detail, nil
}

func (h *DetailHandler) collectBondSection(detail *InterfaceDetail, swIfIndex uint32) {
	bonds, err := h.southbound.DumpBondInterfaces()
	if err != nil {
		return
	}

	for _, b := range bonds {
		if b.SwIfIndex != swIfIndex {
			continue
		}

		section := &BondSection{
			Mode:          b.Mode,
			LoadBalance:   b.LoadBalance,
			Members:       b.Members,
			ActiveMembers: b.ActiveMembers,
		}

		if members, err := h.southbound.DumpBondMembers(swIfIndex); err == nil {
			for _, m := range members {
				section.MemberDetails = append(section.MemberDetails, BondMemberInfo{
					Name:          m.Name,
					SwIfIndex:     m.SwIfIndex,
					IsPassive:     m.IsPassive,
					IsLongTimeout: m.IsLongTimeout,
					IsLocalNuma:   m.IsLocalNuma,
					Weight:        m.Weight,
				})
			}
		}

		if b.Mode == "lacp" {
			if lacpIfaces, err := h.southbound.DumpLACPInterfaces(); err == nil {
				for _, l := range lacpIfaces {
					if l.BondName == detail.Name {
						section.LACP = append(section.LACP, LACPMemberInfo{
							Name:                  l.Name,
							SwIfIndex:             l.SwIfIndex,
							RxState:               l.RxState,
							TxState:               l.TxState,
							MuxState:              l.MuxState,
							PtxState:              l.PtxState,
							ActorSystemPriority:   l.ActorSystemPriority,
							ActorSystem:           l.ActorSystem,
							ActorKey:              l.ActorKey,
							ActorPortPriority:     l.ActorPortPriority,
							ActorPortNumber:       l.ActorPortNumber,
							ActorState:            l.ActorState,
							PartnerSystemPriority: l.PartnerSystemPriority,
							PartnerSystem:         l.PartnerSystem,
							PartnerKey:            l.PartnerKey,
							PartnerPortPriority:   l.PartnerPortPriority,
							PartnerPortNumber:     l.PartnerPortNumber,
							PartnerState:          l.PartnerState,
						})
					}
				}
			}
		}

		detail.Bond = section
		break
	}
}

func (h *DetailHandler) collectSubinterfaceSection(detail *InterfaceDetail, iface *ifmgr.Interface) {
	parentName := ""
	if parent := h.southbound.GetIfMgr().Get(iface.SupSwIfIndex); parent != nil {
		parentName = parent.Name
	}

	detail.Subinterface = &SubinterfaceSection{
		Parent:          parentName,
		SubID:           iface.SubID,
		SubNumberOfTags: iface.SubNumberOfTags,
		OuterVlanID:     iface.OuterVlanID,
		InnerVlanID:     iface.InnerVlanID,
	}
}

func (h *DetailHandler) PathPattern() paths.Path {
	return paths.InterfacesDetail
}

func (h *DetailHandler) Dependencies() []paths.Path {
	return nil
}

func (h *DetailHandler) Summary() string {
	return "Show interface details"
}

func (h *DetailHandler) Description() string {
	return "Display detailed configuration and state for a specific interface by name."
}
