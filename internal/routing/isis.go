package routing

import (
	"encoding/json"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/models/protocols/isis"
)

func (c *Component) GetISISNeighbors() ([]isis.Area, error) {
	output, err := c.execVtysh("-c", "show isis neighbor json")
	if err != nil {
		return nil, err
	}

	var result struct {
		Areas []isis.Area `json:"areas"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("parse ISIS neighbors: %w", err)
	}

	return result.Areas, nil
}
