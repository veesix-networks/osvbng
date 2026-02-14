package servicegroups

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/config/servicegroup"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/svcgroup"
)

func init() {
	conf.RegisterFactory(NewServiceGroupHandler)
}

type ServiceGroupHandler struct {
	resolver *svcgroup.Resolver
}

func NewServiceGroupHandler(d *deps.ConfDeps) conf.Handler {
	return &ServiceGroupHandler{resolver: d.SvcGroupResolver}
}

func (h *ServiceGroupHandler) extractName(path string) (string, error) {
	values, err := paths.ServiceGroups.ExtractWildcards(path, 1)
	if err != nil {
		return "", fmt.Errorf("extract service group name from path: %w", err)
	}
	return values[0], nil
}

func (h *ServiceGroupHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, err := h.extractName(hctx.Path)
	if err != nil {
		return err
	}

	if hctx.NewValue == nil {
		return nil
	}

	if _, ok := hctx.NewValue.(*servicegroup.Config); !ok {
		return fmt.Errorf("expected *servicegroup.Config, got %T", hctx.NewValue)
	}

	return nil
}

func (h *ServiceGroupHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	name, err := h.extractName(hctx.Path)
	if err != nil {
		return err
	}

	if hctx.NewValue == nil {
		h.resolver.Delete(name)
		return nil
	}

	cfg, ok := hctx.NewValue.(*servicegroup.Config)
	if !ok {
		return fmt.Errorf("expected *servicegroup.Config, got %T", hctx.NewValue)
	}

	h.resolver.Set(name, cfg)
	return nil
}

func (h *ServiceGroupHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	name, err := h.extractName(hctx.Path)
	if err != nil {
		return err
	}

	if hctx.OldValue == nil {
		h.resolver.Delete(name)
		return nil
	}

	cfg, ok := hctx.OldValue.(*servicegroup.Config)
	if !ok {
		return nil
	}

	h.resolver.Set(name, cfg)
	return nil
}

func (h *ServiceGroupHandler) PathPattern() paths.Path {
	return paths.ServiceGroups
}

func (h *ServiceGroupHandler) Dependencies() []paths.Path {
	return nil
}

func (h *ServiceGroupHandler) Callbacks() *conf.Callbacks {
	return nil
}
