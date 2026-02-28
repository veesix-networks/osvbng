// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &SRGCountersHandler{deps: d}
	})
}

type SRGCountersHandler struct {
	deps *deps.ShowDeps
}

type SRGCounterDetail struct {
	SRGName    string `json:"srg_name"`
	GarpSent   uint64 `json:"garp_sent"`
	NaSent     uint64 `json:"na_sent"`
	MacAdds    uint64 `json:"mac_adds"`
	MacRemoves uint64 `json:"mac_removes"`
}

func (h *SRGCountersHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	if h.deps.HAManager == nil {
		return []SRGCounterDetail{}, nil
	}

	dp := h.deps.HAManager.GetSRGDataplane()
	if dp == nil {
		return []SRGCounterDetail{}, nil
	}

	counters, err := dp.GetSRGCounters("")
	if err != nil {
		return nil, err
	}

	var result []SRGCounterDetail
	for _, c := range counters {
		result = append(result, SRGCounterDetail{
			SRGName:    c.SRGName,
			GarpSent:   c.GarpSent,
			NaSent:     c.NaSent,
			MacAdds:    c.MacAdds,
			MacRemoves: c.MacRemoves,
		})
	}

	if result == nil {
		result = []SRGCounterDetail{}
	}

	return result, nil
}

func (h *SRGCountersHandler) PathPattern() paths.Path {
	return paths.HASRGCounters
}

func (h *SRGCountersHandler) Dependencies() []paths.Path {
	return nil
}
