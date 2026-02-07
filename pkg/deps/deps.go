package deps

import (
	aaacomp "github.com/veesix-networks/osvbng/internal/aaa"
	routingcomp "github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/cppm"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/operations"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type ShowDeps struct {
	Subscriber       *subscriber.Component
	Southbound       *southbound.VPP
	Routing          *routingcomp.Component
	Cache            cache.Cache
	OpDB             opdb.Store
	CPPM             *cppm.Manager
	PluginComponents map[string]component.Component
}

type OperDeps struct {
	PluginComponents map[string]component.Component
}

type ConfDeps struct {
	Dataplane        operations.Dataplane
	DataplaneState   operations.DataplaneStateReader
	Southbound       *southbound.VPP
	AAA              *aaacomp.Component
	Routing          *routingcomp.Component
	CPPM             *cppm.Manager
	PluginComponents map[string]component.Component
}
