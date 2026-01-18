package component

import (
	"github.com/veesix-networks/osvbng/pkg/cache"
	"github.com/veesix-networks/osvbng/pkg/dataplane"
	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/config"
	"github.com/veesix-networks/osvbng/pkg/southbound"
)

type Dependencies struct {
	EventBus events.Bus
	Cache    cache.Cache
	VPP      *southbound.VPP
	Config   *config.Config

	DHCPChan <-chan *dataplane.ParsedPacket
	ARPChan  <-chan *dataplane.ParsedPacket
	PPPChan  <-chan *dataplane.ParsedPacket
}
