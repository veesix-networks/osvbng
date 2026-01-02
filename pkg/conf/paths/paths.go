package paths

type Path string

const (
	VRFS                        Path = "vrfs.*"
	VRFSName                    Path = "vrfs.*.name"
	VRFSDescription             Path = "vrfs.*.description"
	VRFSRouteRouteDistinguisher Path = "vrfs.*.route-distinguisher"
	VRFSImportRouteTargets      Path = "vrfs.*.import-route-targets"
	VRFSExportRouteTargets      Path = "vrfs.*.export-route-targets"

	Interface            Path = "interfaces.*"
	InterfaceDescription Path = "interfaces.*.description"
	InterfaceMTU         Path = "interfaces.*.mtu"
	InterfaceEnabled     Path = "interfaces.*.enabled"
	InterfaceIPv4Address Path = "interfaces.*.address.ipv4"
	InterfaceIPv6Address Path = "interfaces.*.address.ipv6"

	BFDEnabled Path = "protocols.bfd.enabled"

	OSPFEnabled     Path = "protocols.ospf.enabled"
	OSPFAreaBFD     Path = "protocols.ospf.areas.*.bfd"
	OSPFAreaNetwork Path = "protocols.ospf.areas.*.networks.*"

	ProtocolsBGPInstance     Path = "protocols.bgp.*"
	ProtocolsBGPASN          Path = "protocols.bgp.asn"
	ProtocolsBGPRouterID     Path = "protocols.bgp.router-id"
	ProtocolsBGPEnabled      Path = "protocols.bgp.enabled"
	ProtocolsBGPNeighborBFD  Path = "protocols.bgp.neighbors.*.bfd"
	ProtocolsBGPNeighborPeer Path = "protocols.bgp.neighbors.*.peer"

	ProtocolsStaticRoute Path = "protocols.static"

	AAARADIUSServer  Path = "aaa.radius.servers.*"
	AAARADIUSGroup   Path = "aaa.radius.groups.*"
	AAANASIdentifier Path = "aaa.nas_identifier"
	AAANASIP         Path = "aaa.nas_ip"
)

func (p Path) String() string {
	return string(p)
}
