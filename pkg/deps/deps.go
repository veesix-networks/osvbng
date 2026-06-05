// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package deps

import (
	aaacomp "github.com/veesix-networks/osvbng/internal/aaa"
	cgnatcomp "github.com/veesix-networks/osvbng/internal/cgnat"
	l2tpcomp "github.com/veesix-networks/osvbng/internal/l2tp"
	routingcomp "github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/internal/watchdog"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/cppm"
	"github.com/veesix-networks/osvbng/pkg/dhcp4"
	"github.com/veesix-networks/osvbng/pkg/dhcp6"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ha"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

type RunningConfigReader interface {
	GetRunning() (*config.Config, error)
}

type ShowDeps struct {
	Subscriber       *subscriber.Component
	Southbound       southbound.Southbound
	Routing          *routingcomp.Component
	VRFManager       *vrfmgr.Manager
	SvcGroupResolver *svcgroup.Resolver
	Cache            cache.Cache
	OpDB             opdb.Store
	CPPM             *cppm.Manager
	Watchdog         watchdog.StateProvider
	EventBus         events.Bus
	HAManager        *ha.Manager
	PluginComponents map[string]component.Component
	DHCPv4Providers  map[string]dhcp4.DHCPProvider
	DHCPv6Providers  map[string]dhcp6.DHCPProvider
	CGNAT            *cgnatcomp.Component
	L2TP             *l2tpcomp.Component
	RunningConfig    RunningConfigReader
	Orchestrator     *component.Orchestrator
}

// OperConfigReloader is the narrow read-side contract the oper layer
// needs from configmgr to re-render templates and push the result into
// FRR. Defined here rather than importing *configmgr.ConfigManager
// directly so OperDeps stays free of the configmgr → deps cycle.
// *configmgr.ConfigManager satisfies this implicitly.
type OperConfigReloader interface {
	ReloadFRR() error
}

type OperDeps struct {
	Subscriber       *subscriber.Component
	Southbound       southbound.Southbound
	EventBus         events.Bus
	HAManager        *ha.Manager
	PluginComponents map[string]component.Component
	CGNAT            *cgnatcomp.Component
	ConfigReloader   OperConfigReloader
}

type ConfDeps struct {
	DataplaneState   operations.DataplaneStateReader
	Southbound       southbound.Southbound
	AAA              *aaacomp.Component
	Routing          *routingcomp.Component
	VRFManager       *vrfmgr.Manager
	SvcGroupResolver *svcgroup.Resolver
	CPPM             *cppm.Manager
	PluginComponents map[string]component.Component
}
