package routing

import (
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/models/protocols/ospf"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/ospf6"
)

var ospfVRFNameRE = regexp.MustCompile(`^(all|[A-Za-z0-9_-]+)$`)

func ospfVRFPrefix(vrf string) (string, error) {
	if vrf == "" {
		return "", nil
	}
	if !ospfVRFNameRE.MatchString(vrf) {
		return "", fmt.Errorf("invalid VRF name %q", vrf)
	}
	return "vrf " + vrf + " ", nil
}

func (c *Component) GetOSPFNeighbors() (map[string][]ospf.Neighbor, error) {
	output, err := c.execVtysh("-c", "show ip ospf neighbor json")
	if err != nil {
		return nil, err
	}

	var result struct {
		Neighbors map[string][]ospf.Neighbor `json:"neighbors"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parse OSPF neighbors: %w", err)
	}

	return result.Neighbors, nil
}

func (c *Component) GetOSPFInstance(vrf string) (*ospf.Instance, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show ip ospf "+prefix+"json")
	if err != nil {
		return nil, err
	}
	var inst ospf.Instance
	if err := json.Unmarshal(output, &inst); err != nil {
		return nil, fmt.Errorf("parse OSPF instance: %w", err)
	}
	return &inst, nil
}

func (c *Component) GetOSPFInstanceAll() (map[string]ospf.Instance, error) {
	output, err := c.execVtysh("-c", "show ip ospf vrf all json")
	if err != nil {
		return nil, err
	}
	out := map[string]ospf.Instance{}
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("parse OSPF instance (vrf all): %w", err)
	}
	return out, nil
}

func (c *Component) GetOSPFInterfaces(vrf, iface string) (*ospf.InterfaceMap, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show ip ospf " + prefix + "interface"
	if iface != "" {
		cmd += " " + iface
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	var im ospf.InterfaceMap
	if err := json.Unmarshal(output, &im); err != nil {
		return nil, fmt.Errorf("parse OSPF interfaces: %w", err)
	}
	return &im, nil
}

func (c *Component) GetOSPFInterfacesAll() (map[string]ospf.InterfaceMap, error) {
	output, err := c.execVtysh("-c", "show ip ospf vrf all interface json")
	if err != nil {
		return nil, err
	}
	out := map[string]ospf.InterfaceMap{}
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("parse OSPF interfaces (vrf all): %w", err)
	}
	return out, nil
}

func (c *Component) GetOSPFNeighbor(vrf, routerID string, detail bool) ([]ospf.Neighbor, error) {
	if ip := net.ParseIP(routerID); ip == nil || ip.To4() == nil {
		return nil, fmt.Errorf("invalid OSPF router-id %q", routerID)
	}
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show ip ospf " + prefix + "neighbor " + routerID
	if detail {
		cmd += " detail"
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	var byVRF map[string]map[string][]ospf.Neighbor
	if err := json.Unmarshal(output, &byVRF); err != nil {
		return nil, fmt.Errorf("parse OSPF neighbor: %w", err)
	}
	for _, byRouter := range byVRF {
		if entries, ok := byRouter[routerID]; ok {
			return entries, nil
		}
	}
	return nil, nil
}

func (c *Component) GetOSPFNeighborsDetail(vrf, iface string) (*ospf.NeighborDetailMap, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show ip ospf " + prefix + "neighbor"
	if iface != "" {
		cmd += " " + iface
	}
	cmd += " detail json"
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return nil, err
	}
	var ndm ospf.NeighborDetailMap
	if err := json.Unmarshal(output, &ndm); err != nil {
		return nil, fmt.Errorf("parse OSPF neighbor detail: %w", err)
	}
	return &ndm, nil
}

func (c *Component) GetOSPFNeighborsDetailAll() (map[string]ospf.NeighborDetailMap, error) {
	output, err := c.execVtysh("-c", "show ip ospf vrf all neighbor detail json")
	if err != nil {
		return nil, err
	}
	out := map[string]ospf.NeighborDetailMap{}
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("parse OSPF neighbor detail (vrf all): %w", err)
	}
	return out, nil
}

func (c *Component) GetOSPFGRHelper(vrf string, detail bool) (*ospf.GRHelper, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show ip ospf " + prefix + "graceful-restart helper"
	if detail {
		cmd += " detail"
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	var gr ospf.GRHelper
	if err := json.Unmarshal(output, &gr); err != nil {
		return nil, fmt.Errorf("parse OSPF gr-helper: %w", err)
	}
	return &gr, nil
}

func (c *Component) GetOSPFGRHelperAll() (map[string]ospf.GRHelper, error) {
	output, err := c.execVtysh("-c", "show ip ospf vrf all graceful-restart helper json")
	if err != nil {
		return nil, err
	}
	out := map[string]ospf.GRHelper{}
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("parse OSPF gr-helper (vrf all): %w", err)
	}
	return out, nil
}

func (c *Component) GetOSPFRoute(detail bool) (map[string]ospf.Route, error) {
	cmd := "show ip ospf route"
	if detail {
		cmd += " detail"
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	out := map[string]ospf.Route{}
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("parse OSPF route: %w", err)
	}
	return out, nil
}

func (c *Component) GetOSPFBorderRouters(vrf string) (*ospf.BorderRouterMap, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show ip ospf "+prefix+"border-routers json")
	if err != nil {
		return nil, err
	}
	var brm ospf.BorderRouterMap
	if err := json.Unmarshal(output, &brm); err != nil {
		return nil, fmt.Errorf("parse OSPF border-routers: %w", err)
	}
	return &brm, nil
}

func (c *Component) GetOSPFReachableRouters(vrf string) (string, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return "", err
	}
	output, err := c.execVtysh("-c", "show ip ospf "+prefix+"reachable-routers")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

var validOSPFLSATypes = map[string]struct{}{
	"router":        {},
	"network":       {},
	"summary":       {},
	"asbr-summary":  {},
	"external":      {},
	"nssa-external": {},
	"opaque-link":   {},
	"opaque-area":   {},
	"opaque-as":     {},
}

func ospfValidateIPv4(name, value string) error {
	if ip := net.ParseIP(value); ip == nil || ip.To4() == nil {
		return fmt.Errorf("invalid %s %q", name, value)
	}
	return nil
}

func (c *Component) GetOSPFDatabase(vrf, lsaType string, opts ospf.DatabaseOpts) (json.RawMessage, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show ip ospf " + prefix + "database"
	if lsaType != "" {
		if _, ok := validOSPFLSATypes[lsaType]; !ok {
			return nil, fmt.Errorf("invalid OSPF LSA type %q", lsaType)
		}
		cmd += " " + lsaType
	}
	if opts.MaxAge && lsaType == "" {
		cmd += " max-age"
	}
	if opts.Detail && lsaType == "" {
		cmd += " detail"
	}
	if opts.LinkStateID != "" {
		if err := ospfValidateIPv4("LSA link-state-id", opts.LinkStateID); err != nil {
			return nil, err
		}
		cmd += " " + opts.LinkStateID
	}
	if opts.SelfOriginate {
		cmd += " self-originate"
	}
	if opts.AdvRouter != "" {
		if err := ospfValidateIPv4("LSA adv-router", opts.AdvRouter); err != nil {
			return nil, err
		}
		cmd += " adv-router " + opts.AdvRouter
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

var validMPLSTEDatabaseScopes = map[string]struct{}{
	"vertex": {},
	"edge":   {},
	"subnet": {},
}

func (c *Component) GetOSPFMPLSTEInterface(iface string) (string, error) {
	cmd := "show ip ospf mpls-te interface"
	if iface != "" {
		cmd += " " + iface
	}
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Component) GetOSPFMPLSTERouter() (string, error) {
	output, err := c.execVtysh("-c", "show ip ospf mpls-te router")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Component) GetOSPFMPLSTEDatabase(opts ospf.MPLSTEDatabaseOpts) (string, error) {
	cmd := "show ip ospf mpls-te database"
	if opts.Scope != "" {
		if _, ok := validMPLSTEDatabaseScopes[opts.Scope]; !ok {
			return "", fmt.Errorf("invalid MPLS-TE database scope %q", opts.Scope)
		}
		cmd += " " + opts.Scope
	}
	if opts.AdvRouter != "" {
		if err := ospfValidateIPv4("MPLS-TE adv-router", opts.AdvRouter); err != nil {
			return "", err
		}
		cmd += " adv-router " + opts.AdvRouter
	}
	if opts.LSID != "" {
		cmd += " " + opts.LSID
	}
	if opts.Verbose {
		cmd += " verbose"
	}
	cmd += " json"
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Component) GetOSPFRouterInfo(pce bool) (string, error) {
	cmd := "show ip ospf router-info"
	if pce {
		cmd += " pce"
	}
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Component) GetOSPFSegmentRouting(advRouter string, selfOriginate bool) (string, error) {
	cmd := "show ip ospf database segment-routing"
	switch {
	case selfOriginate:
		cmd += " self-originate"
	case advRouter != "":
		if err := ospfValidateIPv4("SR adv-router", advRouter); err != nil {
			return "", err
		}
		cmd += " adv-router " + advRouter
	}
	cmd += " json"
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Component) GetOSPFSummaryAddress(vrf string, detail bool) (*ospf.SummaryAddress, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show ip ospf " + prefix + "summary-address"
	if detail {
		cmd += " detail"
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	var sa ospf.SummaryAddress
	if err := json.Unmarshal(output, &sa); err != nil {
		return nil, fmt.Errorf("parse OSPF summary-address: %w", err)
	}
	return &sa, nil
}

func (c *Component) GetOSPF6Instance(vrf string) (*ospf6.Instance, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show ipv6 ospf6 "+prefix+"json")
	if err != nil {
		return nil, err
	}
	var inst ospf6.Instance
	if err := json.Unmarshal(output, &inst); err != nil {
		return nil, fmt.Errorf("parse OSPFv3 instance: %w", err)
	}
	return &inst, nil
}

func (c *Component) GetOSPF6Interfaces(vrf, iface string) (map[string]ospf6.Interface, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show ipv6 ospf6 " + prefix + "interface"
	if iface != "" {
		cmd += " " + iface
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	out := map[string]ospf6.Interface{}
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("parse OSPFv3 interfaces: %w", err)
	}
	return out, nil
}

func (c *Component) GetOSPF6InterfaceTraffic(vrf, iface string) (map[string]ospf6.InterfaceTraffic, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show ipv6 ospf6 " + prefix + "interface traffic"
	if iface != "" {
		cmd += " " + iface
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	out := map[string]ospf6.InterfaceTraffic{}
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("parse OSPFv3 interface traffic: %w", err)
	}
	return out, nil
}

func (c *Component) GetOSPF6Neighbor(vrf, routerID string) (json.RawMessage, error) {
	if err := ospfValidateIPv4("OSPFv3 router-id", routerID); err != nil {
		return nil, err
	}
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show ipv6 ospf6 "+prefix+"neighbor "+routerID+" json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetOSPF6NeighborsDetail(vrf string) (json.RawMessage, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show ipv6 ospf6 "+prefix+"neighbor detail json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetOSPF6NeighborsDRChoice(vrf string) (json.RawMessage, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show ipv6 ospf6 "+prefix+"neighbor drchoice json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetOSPF6GRHelper(vrf string, detail bool) (*ospf6.GRHelper, error) {
	prefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show ipv6 ospf6 " + prefix + "graceful-restart helper"
	if detail {
		cmd += " detail"
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	var gr ospf6.GRHelper
	if err := json.Unmarshal(output, &gr); err != nil {
		return nil, fmt.Errorf("parse OSPFv3 gr-helper: %w", err)
	}
	return &gr, nil
}

var validOSPF6LSATypes = map[string]struct{}{
	"router":           {},
	"network":          {},
	"inter-prefix":     {},
	"inter-router":     {},
	"as-external":      {},
	"group-membership": {},
	"type-7":           {},
	"link":             {},
	"intra-prefix":     {},
}

func (c *Component) GetOSPF6Database(vrf, lsaType string, opts ospf6.DatabaseOpts) (json.RawMessage, error) {
	vrfPrefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show ipv6 ospf6 " + vrfPrefix + "database"
	if lsaType != "" {
		if _, ok := validOSPF6LSATypes[lsaType]; !ok {
			return nil, fmt.Errorf("invalid OSPFv3 LSA type %q", lsaType)
		}
		cmd += " " + lsaType
	} else {
		switch {
		case opts.Detail:
			cmd += " detail"
		case opts.Dump:
			cmd += " dump"
		case opts.Internal:
			cmd += " internal"
		}
	}
	if opts.AdvRouter != "" {
		if err := ospfValidateIPv4("LSA adv-router", opts.AdvRouter); err != nil {
			return nil, err
		}
		cmd += " adv-router " + opts.AdvRouter
	}
	if opts.LinkStateID != "" {
		if err := ospfValidateIPv4("LSA linkstate-id", opts.LinkStateID); err != nil {
			return nil, err
		}
		cmd += " linkstate-id " + opts.LinkStateID
	}
	if opts.SelfOriginated {
		cmd += " self-originated"
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

var validOSPF6RouteFilters = map[string]struct{}{
	"intra-area": {},
	"inter-area": {},
	"external-1": {},
	"external-2": {},
	"detail":     {},
	"summary":    {},
}

func (c *Component) GetOSPF6InterfacePrefix(vrf, iface, prefix string, detail, match bool) (json.RawMessage, error) {
	vrfPrefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show ipv6 ospf6 " + vrfPrefix + "interface"
	if iface != "" {
		cmd += " " + iface
	}
	cmd += " prefix"
	if prefix != "" {
		cmd += " " + prefix
		if match {
			cmd += " match"
		}
	}
	if detail {
		cmd += " detail"
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetOSPF6Route(vrf, filter, prefix string, detail, match bool) (*ospf6.RouteResponse, error) {
	vrfPrefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := "show ipv6 ospf6 " + vrfPrefix + "route"
	if filter != "" {
		if _, ok := validOSPF6RouteFilters[filter]; !ok {
			return nil, fmt.Errorf("invalid OSPFv3 route filter %q", filter)
		}
		cmd += " " + filter
	}
	if prefix != "" {
		cmd += " " + prefix
		if match {
			cmd += " match"
		}
	}
	if detail {
		cmd += " detail"
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	var r ospf6.RouteResponse
	if err := json.Unmarshal(output, &r); err != nil {
		return nil, fmt.Errorf("parse OSPFv3 route: %w", err)
	}
	return &r, nil
}

func (c *Component) GetOSPF6SpfTree(vrf string) (json.RawMessage, error) {
	vrfPrefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show ipv6 ospf6 "+vrfPrefix+"spf tree json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetOSPF6Redistribute(vrf string) (json.RawMessage, error) {
	vrfPrefix, err := ospfVRFPrefix(vrf)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show ipv6 ospf6 "+vrfPrefix+"redistribute json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetOSPF6Zebra() (json.RawMessage, error) {
	output, err := c.execVtysh("-c", "show ipv6 ospf6 zebra json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetOSPF6SummaryAddress(detail bool) (json.RawMessage, error) {
	cmd := "show ipv6 ospf6 summary-address"
	if detail {
		cmd += " detail"
	}
	output, err := c.execVtysh("-c", cmd+" json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetOSPF6Neighbors() ([]ospf6.Neighbor, error) {
	output, err := c.execVtysh("-c", "show ipv6 ospf6 neighbor json")
	if err != nil {
		return nil, err
	}

	var result struct {
		Neighbors []ospf6.Neighbor `json:"neighbors"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parse OSPFv3 neighbors: %w", err)
	}

	return result.Neighbors, nil
}
