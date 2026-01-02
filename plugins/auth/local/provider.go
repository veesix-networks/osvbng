package local

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/provider"
)

func init() {
	auth.Register("local", New)
}

type Provider struct{}

func New(cfg map[string]string) (auth.AuthProvider, error) {
	return &Provider{}, nil
}

func (p *Provider) Info() provider.Info {
	return provider.Info{
		Name:    "local",
		Version: "1.0.0",
		Author:  "OSVBNG Core",
	}
}

func (p *Provider) Authenticate(ctx context.Context, req *auth.AuthRequest) (*auth.AuthResponse, error) {
	return &auth.AuthResponse{
		Allowed:    true,
		Attributes: make(map[string]string),
	}, nil
}

func (p *Provider) StartAccounting(ctx context.Context, session *auth.Session) error {
	return nil
}

func (p *Provider) UpdateAccounting(ctx context.Context, session *auth.Session) error {
	return nil
}

func (p *Provider) StopAccounting(ctx context.Context, session *auth.Session) error {
	return nil
}
