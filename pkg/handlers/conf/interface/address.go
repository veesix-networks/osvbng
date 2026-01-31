package iface

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
)

func init() {
	conf.RegisterFactory(NewInterfaceIPv4AddressHandler)
	conf.RegisterFactory(NewInterfaceIPv6AddressHandler)
	conf.RegisterFactory(NewSubinterfaceIPv4AddressHandler)
	conf.RegisterFactory(NewSubinterfaceIPv6AddressHandler)
}

type AddressHandler struct {
	dataplane      operations.Dataplane
	dataplaneState operations.DataplaneStateReader
	pathPattern    paths.Path
	dependencies   []paths.Path
	isIPv6         bool
}

func NewInterfaceIPv4AddressHandler(d *deps.ConfDeps) conf.Handler {
	return &AddressHandler{
		dataplane:      d.Dataplane,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InterfaceIPv4Address,
		dependencies:   []paths.Path{paths.Interface},
		isIPv6:         false,
	}
}

func NewInterfaceIPv6AddressHandler(d *deps.ConfDeps) conf.Handler {
	return &AddressHandler{
		dataplane:      d.Dataplane,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InterfaceIPv6Address,
		dependencies:   []paths.Path{paths.Interface},
		isIPv6:         true,
	}
}

func NewSubinterfaceIPv4AddressHandler(d *deps.ConfDeps) conf.Handler {
	return &AddressHandler{
		dataplane:      d.Dataplane,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InterfaceSubinterfaceIPv4Address,
		dependencies:   []paths.Path{paths.InterfaceSubinterface},
		isIPv6:         false,
	}
}

func NewSubinterfaceIPv6AddressHandler(d *deps.ConfDeps) conf.Handler {
	return &AddressHandler{
		dataplane:      d.Dataplane,
		dataplaneState: d.DataplaneState,
		pathPattern:    paths.InterfaceSubinterfaceIPv6Address,
		dependencies:   []paths.Path{paths.InterfaceSubinterface},
		isIPv6:         true,
	}
}

func (h *AddressHandler) extractInterfaceName(path string) (string, error) {
	values, err := h.pathPattern.ExtractWildcards(path, 2)
	if err != nil {
		return "", err
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return fmt.Sprintf("%s.%s", values[0], values[1]), nil
}

func (h *AddressHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	addr, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("address must be a string")
	}

	ip, _, err := net.ParseCIDR(addr)
	if err != nil {
		return fmt.Errorf("invalid CIDR address: %w", err)
	}

	if h.isIPv6 {
		if ip.To4() != nil || !strings.Contains(addr, ":") {
			return fmt.Errorf("not an IPv6 address")
		}
	} else {
		if ip.To4() == nil {
			return fmt.Errorf("not an IPv4 address")
		}
	}

	return nil
}

func (h *AddressHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName, err := h.extractInterfaceName(hctx.Path)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}
	addr := hctx.NewValue.(string)

	if h.dataplaneState != nil {
		if h.isIPv6 {
			if h.dataplaneState.HasIPv6Address(ifName, addr) {
				return nil
			}
		} else {
			if h.dataplaneState.HasIPv4Address(ifName, addr) {
				return nil
			}
		}
	}

	if h.isIPv6 {
		return h.dataplane.AddIPv6Address(ifName, addr)
	}
	return h.dataplane.AddIPv4Address(ifName, addr)
}

func (h *AddressHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName, err := h.extractInterfaceName(hctx.Path)
	if err != nil {
		return fmt.Errorf("extract interface name: %w", err)
	}
	addr := hctx.NewValue.(string)

	if h.isIPv6 {
		return h.dataplane.DelIPv6Address(ifName, addr)
	}
	return h.dataplane.DelIPv4Address(ifName, addr)
}

func (h *AddressHandler) PathPattern() paths.Path {
	return h.pathPattern
}

func (h *AddressHandler) Dependencies() []paths.Path {
	return h.dependencies
}

func (h *AddressHandler) Callbacks() *conf.Callbacks {
	return nil
}
