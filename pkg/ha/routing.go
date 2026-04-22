// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/config"
)

type RoutingController interface {
	AdvertiseSRGNetworks(ctx context.Context, networks []config.SRGNetwork) error
	WithdrawSRGNetworks(ctx context.Context, networks []config.SRGNetwork) error
}

func WithRoutingController(rc RoutingController) ManagerOption {
	return func(m *Manager) { m.routingCtrl = rc }
}
