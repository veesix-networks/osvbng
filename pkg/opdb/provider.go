package opdb

import "context"

type Provider interface {
	Namespaces() []string
	Restore(ctx context.Context, store Store) error
}

type ProviderRegistry struct {
	providers []Provider
}

func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{}
}

func (r *ProviderRegistry) Register(p Provider) {
	r.providers = append(r.providers, p)
}

func (r *ProviderRegistry) RestoreAll(ctx context.Context, store Store) error {
	for _, p := range r.providers {
		if err := p.Restore(ctx, store); err != nil {
			return err
		}
	}
	return nil
}
