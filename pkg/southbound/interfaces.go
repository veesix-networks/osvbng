package southbound

import (
	"github.com/veesix-networks/osvbng/pkg/config/interfaces"
	"github.com/veesix-networks/osvbng/pkg/ifmgr"
)

type SubinterfaceParams struct {
	ParentIface  string
	SubID        uint16
	OuterVLAN    uint16
	InnerVLAN    *uint16
	InnerVLANAny bool
	VLANTpid     string
	MSSClamp     *MSSClampPolicy
}

type Interfaces interface {
	CreateSubinterface(params *SubinterfaceParams) error
	DeleteInterface(name string) error

	CreateLCPPair(ifName string) error
	HasLCPPair(ifName string) bool
	BindInterfaceToVRF(vppIfName, vrfName string, hasLCP bool) error

	GetInterfaceIndex(name string) (int, error)
	SetInterfacePromiscuous(ifaceName string, on bool) error

	SetUnnumbered(ifaceName, loopbackName string) error
	SetUnnumberedAsync(swIfIndex uint32, loopbackName string, callback func(error))

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

	// Bond interface queries
	DumpBondInterfaces() ([]BondInterfaceInfo, error)
	DumpBondMembers(bondSwIfIndex uint32) ([]BondMemberInfo, error)
	DumpLACPInterfaces() ([]LACPInterfaceInfo, error)
}
