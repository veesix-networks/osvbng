// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package monitor

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

// Component is the monitoring lifecycle owner. The legacy CollectorRegistry
// / CachedShowCollector pipeline was retired in osvbng-context #59;
// metric emission now flows through pkg/telemetry.RegisterMetric in the
// show handlers themselves.
type Component struct {
	*component.Base
	logger *logger.Logger
}

type Config struct{}

func New(cfg Config) *Component {
	return &Component{
		Base:   component.NewBase("monitor"),
		logger: logger.Get("monitor"),
	}
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting monitoring component")
	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping monitoring component")
	c.StopContext()
	return nil
}
