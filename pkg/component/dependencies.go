package component

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/cppm"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"github.com/veesix-networks/osvbng/pkg/session"
	"github.com/veesix-networks/osvbng/pkg/southbound"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
	"github.com/veesix-networks/osvbng/pkg/vrfmgr"
)

// ShowSource is the narrow read-side contract a component needs to fetch a
// cached snapshot from the show registry without importing it (which would
// cycle through pkg/deps → internal/*).
type ShowSource interface {
	Snapshot(ctx context.Context, path string) (any, error)
}

type ConfigManager interface {
	GetRunning() (*config.Config, error)
	GetStartup() (*config.Config, error)
	LookupSubscriberGroup(svlan, cvlan uint16) (subscriber.GroupMatch, bool)
}

type Dependencies struct {
	EventBus         events.Bus
	Cache            cache.Cache
	Southbound       southbound.Southbound
	VRFManager       *vrfmgr.Manager
	SvcGroupResolver *svcgroup.Resolver
	ConfigManager    ConfigManager
	OpDB             opdb.Store
	CPPM             *cppm.Manager
	Exclusivity      session.ExclusivityRegistry
	AccessResolver   subscriber.AccessResolver
	ShowSource       ShowSource

	DHCPChan   <-chan *dataplane.ParsedPacket
	DHCPv6Chan <-chan *dataplane.ParsedPacket
	ARPChan    <-chan *dataplane.ParsedPacket
	PPPChan    <-chan *dataplane.ParsedPacket
	IPv6NDChan <-chan *dataplane.ParsedPacket
}
