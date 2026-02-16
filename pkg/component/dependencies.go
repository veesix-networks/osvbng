package component

import (
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/cppm"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/southbound/vpp"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

type ConfigManager interface {
	GetRunning() (*config.Config, error)
	GetStartup() (*config.Config, error)
}

type Dependencies struct {
	EventBus      events.Bus
	Cache         cache.Cache
	VPP           *vpp.VPP
	VRFManager       *vrfmgr.Manager
	SvcGroupResolver *svcgroup.Resolver
	ConfigManager ConfigManager
	OpDB          opdb.Store
	CPPM          *cppm.Manager

	DHCPChan    <-chan *dataplane.ParsedPacket
	DHCPv6Chan  <-chan *dataplane.ParsedPacket
	ARPChan     <-chan *dataplane.ParsedPacket
	PPPChan     <-chan *dataplane.ParsedPacket
	IPv6NDChan  <-chan *dataplane.ParsedPacket
}
