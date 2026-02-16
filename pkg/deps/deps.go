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
	"github.com/veesix-networks/osvbng/pkg/southbound/vpp"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

type ShowDeps struct {
	Subscriber       *subscriber.Component
	Southbound       *vpp.VPP
	Routing          *routingcomp.Component
	VRFManager       *vrfmgr.Manager
	SvcGroupResolver *svcgroup.Resolver
	Cache            cache.Cache
	OpDB             opdb.Store
	CPPM             *cppm.Manager
	PluginComponents map[string]component.Component
}

type OperDeps struct {
	Subscriber       *subscriber.Component
	PluginComponents map[string]component.Component
}

type ConfDeps struct {
	Dataplane        operations.Dataplane
	DataplaneState   operations.DataplaneStateReader
	Southbound       *vpp.VPP
	AAA              *aaacomp.Component
	Routing          *routingcomp.Component
	VRFManager       *vrfmgr.Manager
	SvcGroupResolver *svcgroup.Resolver
	CPPM             *cppm.Manager
	PluginComponents map[string]component.Component
}
