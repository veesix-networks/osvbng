// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package dhcp

import (
	"context"
	"time"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/dhcp/relay"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/state"
	statepaths "github.com/veesix-networks/osvbng/pkg/state/paths"
)

const defaultDeadTime = 30 * time.Second

func init() {
	show.RegisterFactory(func(d *deps.ShowDeps) show.ShowHandler {
		return &RelayHandler{}
	})

	state.RegisterMetric(statepaths.DHCPRelay, paths.DHCPRelay)
}

type RelayHandler struct{}

type RelayInfo struct {
	Stats   relay.ClientStats    `json:"stats"`
	Servers []relay.ServerStatus `json:"servers"`
}

func (h *RelayHandler) Collect(_ context.Context, _ *show.Request) (interface{}, error) {
	client := relay.GetClient()
	return &RelayInfo{
		Stats:   client.GetStats(),
		Servers: client.GetServers(defaultDeadTime),
	}, nil
}

func (h *RelayHandler) PathPattern() paths.Path {
	return paths.DHCPRelay
}

func (h *RelayHandler) Dependencies() []paths.Path {
	return nil
}

func (h *RelayHandler) Summary() string {
	return "Show DHCP relay stats and servers"
}

func (h *RelayHandler) Description() string {
	return "Display DHCP relay client statistics and the reachability status of configured relay servers."
}
