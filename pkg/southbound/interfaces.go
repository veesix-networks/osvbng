package southbound

import "github.com/veesix-networks/osvbng/pkg/ifmgr"

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
}
