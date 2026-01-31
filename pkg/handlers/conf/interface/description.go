package iface

import (
	"context"
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
)

func init() {
	conf.RegisterFactory(NewInterfaceDescriptionHandler)
	conf.RegisterFactory(NewSubinterfaceDescriptionHandler)
}

type DescriptionHandler struct {
	dataplane    operations.Dataplane
	pathPattern  paths.Path
	dependencies []paths.Path
}

func NewInterfaceDescriptionHandler(d *deps.ConfDeps) conf.Handler {
	return &DescriptionHandler{
		dataplane:    d.Dataplane,
		pathPattern:  paths.InterfaceDescription,
		dependencies: []paths.Path{paths.Interface},
	}
}

func NewSubinterfaceDescriptionHandler(d *deps.ConfDeps) conf.Handler {
	return &DescriptionHandler{
		dataplane:    d.Dataplane,
		pathPattern:  paths.InterfaceSubinterfaceDescription,
		dependencies: []paths.Path{paths.InterfaceSubinterface},
	}
}

func (h *DescriptionHandler) extractInterfaceName(path string) (string, error) {
	values, err := h.pathPattern.ExtractWildcards(path, 2)
	if err != nil {
		return "", err
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return fmt.Sprintf("%s.%s", values[0], values[1]), nil
}

func (h *DescriptionHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	desc, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("description must be a string")
	}

	if len(desc) > 255 {
		return fmt.Errorf("description too long (max 255 characters)")
	}

	return nil
}

func (h *DescriptionHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName, err := h.extractInterfaceName(hctx.Path)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}
	desc := hctx.NewValue.(string)

	return h.dataplane.SetInterfaceDescription(ifName, desc)
}

func (h *DescriptionHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName, err := h.extractInterfaceName(hctx.Path)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}
	oldDesc := ""
	if hctx.OldValue != nil {
		oldDesc = hctx.OldValue.(string)
	}

	return h.dataplane.SetInterfaceDescription(ifName, oldDesc)
}

func (h *DescriptionHandler) PathPattern() paths.Path {
	return h.pathPattern
}

func (h *DescriptionHandler) Dependencies() []paths.Path {
	return h.dependencies
}

func (h *DescriptionHandler) Callbacks() *conf.Callbacks {
	return nil
}
