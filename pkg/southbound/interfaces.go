package southbound

import (
	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
)

type Interfaces interface {
	CreateSVLAN(parentIface string, vlan uint16, ipv4 []string, ipv6 []string) error
	DeleteInterface(name string) error

	GetInterfaceIndex(name string) (int, error)
	SetInterfacePromiscuous(ifaceName string, on bool) error

	SetUnnumbered(ifaceName, loopbackName string) error

	DumpInterfaces() ([]InterfaceInfo, error)
	DumpUnnumbered() ([]UnnumberedInfo, error)
	DumpIPAddresses() ([]IPAddressInfo, error)

	LoadInterfaces() error
	LoadIPState() error
	GetIfMgr() *ifmgr.Manager

	// Dataplane interface configuration
	CreateInterface(cfg *interfaces.InterfaceConfig) error
	SetInterfaceDescription(name, description string) error
	SetInterfaceMTU(name string, mtu int) error
	SetInterfaceEnabled(name string, enabled bool) error
	AddIPv4Address(ifName, address string) error
	DelIPv4Address(ifName, address string) error
	AddIPv6Address(ifName, address string) error
	DelIPv6Address(ifName, address string) error
}
