// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dataplane

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

func init() {
	show.RegisterFactory(NewBondHandler)
}

type BondHandler struct {
	southbound southbound.Southbound
}

func NewBondHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &BondHandler{southbound: deps.Southbound}
}

type BondInfo struct {
	SwIfIndex     uint32              `json:"sw_if_index"`
	Name          string              `json:"name"`
	Mode          string              `json:"mode"`
	LoadBalance   string              `json:"load_balance"`
	Members       uint32              `json:"members"`
	ActiveMembers uint32              `json:"active_members"`
	MemberDetails []BondMemberDetail  `json:"member_details,omitempty"`
}

type BondMemberDetail struct {
	SwIfIndex     uint32 `json:"sw_if_index"`
	Name          string `json:"name"`
	IsPassive     bool   `json:"is_passive"`
	IsLongTimeout bool   `json:"is_long_timeout"`
	IsLocalNuma   bool   `json:"is_local_numa"`
	Weight        uint32 `json:"weight"`
}

func (h *BondHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	bonds, err := h.southbound.DumpBondInterfaces()
	if err != nil {
		return nil, err
	}

	var result []BondInfo
	for _, b := range bonds {
		info := BondInfo{
			SwIfIndex:     b.SwIfIndex,
			Name:          b.Name,
			Mode:          b.Mode,
			LoadBalance:   b.LoadBalance,
			Members:       b.Members,
			ActiveMembers: b.ActiveMembers,
		}

		members, err := h.southbound.DumpBondMembers(b.SwIfIndex)
		if err == nil {
			for _, m := range members {
				info.MemberDetails = append(info.MemberDetails, BondMemberDetail{
					SwIfIndex:     m.SwIfIndex,
					Name:          m.Name,
					IsPassive:     m.IsPassive,
					IsLongTimeout: m.IsLongTimeout,
					IsLocalNuma:   m.IsLocalNuma,
					Weight:        m.Weight,
				})
			}
		}

		result = append(result, info)
	}

	return result, nil
}

func (h *BondHandler) PathPattern() paths.Path {
	return paths.SystemDataplaneBond
}

func (h *BondHandler) Dependencies() []paths.Path {
	return nil
}
