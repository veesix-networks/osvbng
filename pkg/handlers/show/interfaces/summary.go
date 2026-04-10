// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package interfaces

import (
	"context"
	"sort"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	show.RegisterFactory(NewSummaryHandler)
}

type SummaryHandler struct {
	southbound southbound.Southbound
}

func NewSummaryHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &SummaryHandler{southbound: deps.Southbound}
}

func (h *SummaryHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	all := h.southbound.GetIfMgr().List()

	statsMap := make(map[uint32]*southbound.InterfaceStats)
	if stats, err := h.southbound.GetInterfaceStats(); err == nil {
		for i := range stats {
			statsMap[stats[i].Index] = &stats[i]
		}
	}

	_, includeAll := req.Options["all"]

	var result []InterfaceSummary
	for _, iface := range all {
		if !includeAll && iface.Type == ifmgr.IfTypeP2P {
			continue
		}

		s := InterfaceSummary{
			Name:    iface.Name,
			AdminUp: iface.AdminUp,
			LinkUp:  iface.LinkUp,
			MTU:     iface.MTU,
			Type:    deriveType(iface),
		}

		if st := statsMap[iface.SwIfIndex]; st != nil {
			s.RxPackets = st.Rx
			s.RxBytes = st.RxBytes
			s.RxErrors = st.RxErrors
			s.TxPackets = st.Tx
			s.TxBytes = st.TxBytes
			s.TxErrors = st.TxErrors
			s.Drops = st.Drops
		}

		result = append(result, s)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func deriveType(iface *ifmgr.Interface) string {
	switch iface.Type {
	case ifmgr.IfTypeSub:
		return "sub"
	case ifmgr.IfTypeP2P:
		return "p2p"
	case ifmgr.IfTypePipe:
		return "pipe"
	default:
		if iface.DevType != "" {
			return iface.DevType
		}
		return "hardware"
	}
}

func (h *SummaryHandler) PathPattern() paths.Path {
	return paths.Interfaces
}

func (h *SummaryHandler) Dependencies() []paths.Path {
	return nil
}

func (h *SummaryHandler) Summary() string {
	return "Show all interfaces"
}

func (h *SummaryHandler) Description() string {
	return "List all interfaces with their operational state, addresses, and VPP interface index."
}

type SummaryOptions struct {
	All bool `query:"all" description:"Include point-to-point interfaces"`
}

func (h *SummaryHandler) OptionsType() interface{} {
	return &SummaryOptions{}
}
