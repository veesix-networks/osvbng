package nic

type Generic struct{}

func (g Generic) Name() string { return "Generic" }

func (g Generic) Match(vendorID string) bool {
	return true
}

func (g Generic) BindStrategy() BindStrategy {
	return BindStrategyVFIO
}
