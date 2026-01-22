package nic

type Intel struct{}

func (i Intel) Name() string { return "Intel" }

func (i Intel) Match(vendorID string) bool {
	return vendorID == "8086"
}

func (i Intel) BindStrategy() BindStrategy {
	return BindStrategyVFIO
}
