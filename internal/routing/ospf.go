package routing

import (
	"encoding/json"
	"fmt"
	"regexp"

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
