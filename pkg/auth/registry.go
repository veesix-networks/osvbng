package auth

import "github.com/veesix-networks/osvbng/pkg/provider"

var registry = provider.NewRegistry()

func Register(name string, factory func(cfg map[string]string) (AuthProvider, error)) {
	registry.Register(name, func(cfg map[string]string) (provider.Provider, error) {
		return factory(cfg)
	})
}

func Get(name string) (func(cfg map[string]string) (AuthProvider, error), bool) {
	factory, exists := registry.Get(name)
	if !exists {
		return nil, false
	}

	return func(cfg map[string]string) (AuthProvider, error) {
		p, err := factory(cfg)
		if err != nil {
			return nil, err
		}
		return p.(AuthProvider), nil
	}, true
}

func List() []string {
	return registry.List()
}
