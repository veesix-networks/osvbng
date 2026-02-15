package routing

import (
	"encoding/json"
	"fmt"
)

func (c *Component) GetLDPNeighbors() (interface{}, error) {
	output, err := c.execVtysh("-c", "show mpls ldp neighbor json")
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parse LDP neighbors: %w", err)
	}

	return result, nil
}

func (c *Component) GetLDPBindings() (interface{}, error) {
	output, err := c.execVtysh("-c", "show mpls ldp binding json")
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parse LDP bindings: %w", err)
	}

	return result, nil
}

func (c *Component) GetLDPDiscovery() (interface{}, error) {
	output, err := c.execVtysh("-c", "show mpls ldp discovery json")
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parse LDP discovery: %w", err)
	}

	return result, nil
}
