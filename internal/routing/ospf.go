package routing

import (
	"encoding/json"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/models/protocols/ospf"
	"github.com/veesix-networks/osvbng/pkg/models/protocols/ospf6"
)

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
