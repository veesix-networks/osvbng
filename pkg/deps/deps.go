package deps

import (
	aaacomp "github.com/veesix-networks/osvbng/internal/aaa"
	routingcomp "github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/internal/watchdog"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/cppm"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

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
	PluginComponents map[string]component.Component
}

type OperDeps struct {
	Subscriber       *subscriber.Component
	EventBus         events.Bus
	PluginComponents map[string]component.Component
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
