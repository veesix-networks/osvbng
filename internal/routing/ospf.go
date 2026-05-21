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
