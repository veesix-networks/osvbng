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
	conf.RegisterFactory(NewInterfaceEnabledHandler)
	conf.RegisterFactory(NewSubinterfaceEnabledHandler)
}

type EnabledHandler struct {
	dataplane      operations.Dataplane
	dataplaneState operations.DataplaneStateReader
	pathPattern    paths.Path
	dependencies   []paths.Path
}

func NewInterfaceEnabledHandler(d *deps.ConfDeps) conf.Handler {
	return &EnabledHandler{
		dataplane:      d.Dataplane,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InterfaceEnabled,
		dependencies:   []paths.Path{paths.Interface},
	}
}

func NewSubinterfaceEnabledHandler(d *deps.ConfDeps) conf.Handler {
	return &EnabledHandler{
		dataplane:      d.Dataplane,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InterfaceSubinterfaceEnabled,
		dependencies:   []paths.Path{paths.InterfaceSubinterface},
	}
}

func (h *EnabledHandler) extractInterfaceName(path string) (string, error) {
	values, err := h.pathPattern.ExtractWildcards(path, 2)
	if err != nil {
		return "", err
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return fmt.Sprintf("%s.%s", values[0], values[1]), nil
}

func (h *EnabledHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	_, ok := hctx.NewValue.(bool)
	if !ok {
		return fmt.Errorf("enabled must be a boolean")
	}
	return nil
}

func (h *EnabledHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName, err := h.extractInterfaceName(hctx.Path)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}
	enabled := hctx.NewValue.(bool)

	if h.dataplaneState != nil {
		currentEnabled := h.dataplaneState.IsInterfaceEnabled(ifName)
		if currentEnabled == enabled {
			return nil
		}
	}

	return h.dataplane.SetInterfaceEnabled(ifName, enabled)
}

func (h *EnabledHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName, err := h.extractInterfaceName(hctx.Path)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}

	if hctx.OldValue == nil {
		return nil
	}

	oldEnabled := hctx.OldValue.(bool)
	return h.dataplane.SetInterfaceEnabled(ifName, oldEnabled)
}

func (h *EnabledHandler) PathPattern() paths.Path {
	return h.pathPattern
}

func (h *EnabledHandler) Dependencies() []paths.Path {
	return h.dependencies
}

func (h *EnabledHandler) Callbacks() *conf.Callbacks {
	return nil
}
