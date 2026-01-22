package operations

import (
	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/config/protocols"
)

type Dataplane interface {
	CreateInterface(cfg *interfaces.InterfaceConfig) error
	DeleteInterface(name string) error
	SetInterfaceDescription(name, description string) error
	SetInterfaceMTU(name string, mtu int) error
	SetInterfaceEnabled(name string, enabled bool) error
	AddIPv4Address(ifName, address string) error
	DelIPv4Address(ifName, address string) error
	AddIPv6Address(ifName, address string) error
	DelIPv6Address(ifName, address string) error
	AddRoute(route *protocols.StaticRoute) error
	DelRoute(route *protocols.StaticRoute) error
}
