package nic

type Mellanox struct{}

func (m Mellanox) Name() string { return "Mellanox" }

func (m Mellanox) Match(vendorID string) bool {
	return vendorID == "15b3"
}

func (m Mellanox) BindStrategy() BindStrategy {
	return BindStrategyBifurcated
}
