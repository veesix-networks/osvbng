package paths

import (
	"github.com/veesix-networks/osvbng/pkg/paths"
)

type Path string

const (
	VRFS                        Path = "vrfs.<*>"
	VRFSName                    Path = "vrfs.<*>.name"
	VRFSDescription             Path = "vrfs.<*>.description"
	VRFSRouteRouteDistinguisher Path = "vrfs.<*>.route-distinguisher"
	VRFSImportRouteTargets      Path = "vrfs.<*>.import-route-targets"
	VRFSExportRouteTargets      Path = "vrfs.<*>.export-route-targets"

	Interface            Path = "interfaces.<*>"
	InterfaceDescription Path = "interfaces.<*>.description"
	InterfaceMTU         Path = "interfaces.<*>.mtu"
	InterfaceEnabled     Path = "interfaces.<*>.enabled"
	InterfaceIPv4Address Path = "interfaces.<*>.address.ipv4"
	InterfaceIPv6Address Path = "interfaces.<*>.address.ipv6"

	BFDEnabled Path = "protocols.bfd.enabled"

	OSPFEnabled     Path = "protocols.ospf.enabled"
	OSPFAreaBFD     Path = "protocols.ospf.areas.<*>.bfd"
	OSPFAreaNetwork Path = "protocols.ospf.areas.<*>.networks.<*>"

	ProtocolsBGPInstance         Path = "protocols.bgp"
	ProtocolsBGPASN              Path = "protocols.bgp.asn"
	ProtocolsBGPRouterID         Path = "protocols.bgp.router-id"
	ProtocolsBGPEnabled          Path = "protocols.bgp.enabled"
	ProtocolsBGPNeighborBFD         Path = "protocols.bgp.neighbors.<*:ip>.bfd"
	ProtocolsBGPNeighborDescription Path = "protocols.bgp.neighbors.<*:ip>.description"
	ProtocolsBGPNeighborPeer        Path = "protocols.bgp.neighbors.<*:ip>.peer"
	ProtocolsBGPNeighborRemoteAS    Path = "protocols.bgp.neighbors.<*:ip>.remote_as"
	ProtocolsBGPPeerGroup                Path = "protocols.bgp.peer-groups.<*>"
	ProtocolsBGPIPv4UnicastNetwork       Path = "protocols.bgp.ipv4-unicast.networks.<*:prefix>"
	ProtocolsBGPIPv4UnicastRedistribute  Path = "protocols.bgp.ipv4-unicast.redistribute"
	ProtocolsBGPIPv6UnicastNetwork       Path = "protocols.bgp.ipv6-unicast.networks.<*:prefix>"
	ProtocolsBGPIPv6UnicastRedistribute  Path = "protocols.bgp.ipv6-unicast.redistribute"
	ProtocolsBGPVRFIPv4UnicastNetwork    Path = "protocols.bgp.vrf.<*>.ipv4-unicast.networks.<*:prefix>"
	ProtocolsBGPVRFIPv4UnicastRedistribute Path = "protocols.bgp.vrf.<*>.ipv4-unicast.redistribute"
	ProtocolsBGPVRFIPv6UnicastNetwork    Path = "protocols.bgp.vrf.<*>.ipv6-unicast.networks.<*:prefix>"
	ProtocolsBGPVRFIPv6UnicastRedistribute Path = "protocols.bgp.vrf.<*>.ipv6-unicast.redistribute"

	ProtocolsStaticIPv4Route Path = "protocols.static.ipv4.<*:prefix>"
	ProtocolsStaticIPv6Route Path = "protocols.static.ipv6.<*:prefix>"

	AAARADIUSServer  Path = "aaa.radius.servers.<*>"
	AAARADIUSGroup   Path = "aaa.radius.groups.<*>"
	AAANASIdentifier Path = "aaa.nas_identifier"
	AAANASIP         Path = "aaa.nas_ip"
)

func (p Path) String() string {
	return string(p)
}

func (p Path) Build(values ...string) (string, error) {
	return paths.Build(string(p), values...)
}

func (p Path) ExtractWildcards(path string, expectedCount int) ([]string, error) {
	return paths.Extract(path, string(p))
}

func Build(pattern Path, values ...string) (string, error) {
	return paths.Build(string(pattern), values...)
}

func Extract(path string, pattern Path) ([]string, error) {
	return paths.Extract(path, string(pattern))
}
