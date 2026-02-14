package paths

import (
	"github.com/veesix-networks/osvbng/pkg/paths"
)

type Path string

const (
	ServiceGroups               Path = "service-groups.<*>"
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

	InterfaceSubinterface             Path = "interfaces.<*>.subinterfaces.<*>"
	InterfaceSubinterfaceDescription  Path = "interfaces.<*>.subinterfaces.<*>.description"
	InterfaceSubinterfaceEnabled      Path = "interfaces.<*>.subinterfaces.<*>.enabled"
	InterfaceSubinterfaceMTU          Path = "interfaces.<*>.subinterfaces.<*>.mtu"
	InterfaceSubinterfaceIPv4Address  Path = "interfaces.<*>.subinterfaces.<*>.address.ipv4"
	InterfaceSubinterfaceIPv6Address  Path = "interfaces.<*>.subinterfaces.<*>.address.ipv6"
	InterfaceSubinterfaceIPv6         Path = "interfaces.<*>.subinterfaces.<*>.ipv6"
	InterfaceSubinterfaceARP          Path = "interfaces.<*>.subinterfaces.<*>.arp"
	InterfaceSubinterfaceUnnumbered   Path = "interfaces.<*>.subinterfaces.<*>.unnumbered"
	InterfaceSubinterfaceBNG          Path = "interfaces.<*>.subinterfaces.<*>.bng"

	InterfaceIPv6          Path = "interfaces.<*>.ipv6"
	InterfaceIPv6Enabled   Path = "interfaces.<*>.ipv6.enabled"
	InterfaceIPv6RA        Path = "interfaces.<*>.ipv6.ra"
	InterfaceIPv6Multicast Path = "interfaces.<*>.ipv6.multicast"
	InterfaceARP           Path = "interfaces.<*>.arp"
	InterfaceUnnumbered    Path = "interfaces.<*>.unnumbered"

	BFDEnabled Path = "protocols.bfd.enabled"

	OSPFEnabled              Path = "protocols.ospf.enabled"
	OSPFRouterID             Path = "protocols.ospf.router-id"
	OSPFLogAdjacencyChanges  Path = "protocols.ospf.log-adjacency-changes"
	OSPFAutoCostRefBandwidth Path = "protocols.ospf.auto-cost-reference-bandwidth"
	OSPFMaximumPaths         Path = "protocols.ospf.maximum-paths"
	OSPFDefaultMetric        Path = "protocols.ospf.default-metric"
	OSPFDistance             Path = "protocols.ospf.distance"
	OSPFAreaAuthentication   Path = "protocols.ospf.areas.<*>.authentication"
	OSPFAreaInterface        Path = "protocols.ospf.areas.<*>.interfaces.<*>"
	OSPFRedistribute         Path = "protocols.ospf.redistribute"
	OSPFDefaultInformation   Path = "protocols.ospf.default-information"

	OSPF6Enabled              Path = "protocols.ospf6.enabled"
	OSPF6RouterID             Path = "protocols.ospf6.router-id"
	OSPF6LogAdjacencyChanges  Path = "protocols.ospf6.log-adjacency-changes"
	OSPF6AutoCostRefBandwidth Path = "protocols.ospf6.auto-cost-reference-bandwidth"
	OSPF6MaximumPaths         Path = "protocols.ospf6.maximum-paths"
	OSPF6Distance             Path = "protocols.ospf6.distance"
	OSPF6AreaInterface        Path = "protocols.ospf6.areas.<*>.interfaces.<*>"
	OSPF6Redistribute         Path = "protocols.ospf6.redistribute"
	OSPF6DefaultInformation   Path = "protocols.ospf6.default-information"

	ISISEnabled             Path = "protocols.isis.enabled"
	ISISNET                 Path = "protocols.isis.net"
	ISISIsType              Path = "protocols.isis.is-type"
	ISISMetricStyle         Path = "protocols.isis.metric-style"
	ISISLogAdjacencyChanges Path = "protocols.isis.log-adjacency-changes"
	ISISDynamicHostname     Path = "protocols.isis.dynamic-hostname"
	ISISSetOverloadBit      Path = "protocols.isis.set-overload-bit"
	ISISLSPMTU              Path = "protocols.isis.lsp-mtu"
	ISISLSPGenInterval      Path = "protocols.isis.lsp-gen-interval"
	ISISLSPRefreshInterval  Path = "protocols.isis.lsp-refresh-interval"
	ISISMaxLSPLifetime      Path = "protocols.isis.max-lsp-lifetime"
	ISISSPFInterval         Path = "protocols.isis.spf-interval"
	ISISAreaPassword        Path = "protocols.isis.area-password"
	ISISDomainPassword      Path = "protocols.isis.domain-password"
	ISISRedistribute        Path = "protocols.isis.redistribute"
	ISISDefaultInformation  Path = "protocols.isis.default-information"
	ISISInterface           Path = "protocols.isis.interfaces.<*>"

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

	InternalPuntARP         Path = "_internal.punt.<*>.arp"
	InternalPuntDHCPv4      Path = "_internal.punt.<*>.dhcpv4"
	InternalPuntDHCPv6      Path = "_internal.punt.<*>.dhcpv6"
	InternalPuntPPPoE       Path = "_internal.punt.<*>.pppoe"
	InternalPuntIPv6ND      Path = "_internal.punt.<*>.ipv6nd"
	InternalIPv6Enabled     Path = "_internal.ipv6.<*>.enabled"
	InternalIPv6RA          Path = "_internal.ipv6.<*>.ra"
	InternalIPv6Multicast   Path = "_internal.ipv6.<*>.multicast"
	InternalSVLAN           Path = "_internal.svlan.<*>.<*>"
	InternalUnnumbered      Path = "_internal.unnumbered.<*>"
	InternalDisableARPReply Path = "_internal.disable-arp-reply.<*>"

	SystemCPPMDataplanePolicer    Path = "system.cppm.dataplane.policer.<*>"
	SystemCPPMControlplanePolicer Path = "system.cppm.controlplane.policer.<*>"
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
