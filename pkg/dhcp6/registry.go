package dhcp6

import "github.com/veesix-networks/osvbng/pkg/config"

var factories = make(map[string]func(cfg *config.Config) (DHCPProvider, error))

func Register(name string, factory func(cfg *config.Config) (DHCPProvider, error)) {
	factories[name] = factory
}

func Get(name string) (func(cfg *config.Config) (DHCPProvider, error), bool) {
	factory, exists := factories[name]
	return factory, exists
}

func List() []string {
	names := make([]string, 0, len(factories))
	for name := range factories {
		names = append(names, name)
	}
	return names
}
