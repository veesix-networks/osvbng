package iface

import (
	"github.com/veesix-networks/osvbng/pkg/deps"
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf/paths"
	"github.com/veesix-networks/osvbng/pkg/operations"
)

func init() {
	conf.RegisterFactory(NewIPv4AddressHandler)
	conf.RegisterFactory(NewIPv6AddressHandler)
}

type IPv4AddressHandler struct {
	dataplane operations.Dataplane
}

func NewIPv4AddressHandler(daemons *deps.ConfDeps) conf.Handler {
	return &IPv4AddressHandler{dataplane: daemons.Dataplane}
}

func (h *IPv4AddressHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	addr, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("address must be a string")
	}

	ip, _, err := net.ParseCIDR(addr)
	if err != nil {
		return fmt.Errorf("invalid CIDR address: %w", err)
	}

	if ip.To4() == nil {
		return fmt.Errorf("not an IPv4 address")
	}

	return nil
}

func (h *IPv4AddressHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName := conf.ExtractInterfaceName(hctx.Path)
	addr := hctx.NewValue.(string)

	return h.dataplane.AddIPv4Address(ifName, addr)
}

func (h *IPv4AddressHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName := conf.ExtractInterfaceName(hctx.Path)
	addr := hctx.NewValue.(string)

	return h.dataplane.DelIPv4Address(ifName, addr)
}

func (h *IPv4AddressHandler) PathPattern() paths.Path {
	return paths.InterfaceIPv4Address
}

func (h *IPv4AddressHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.Interface}
}

func (h *IPv4AddressHandler) Callbacks() *conf.Callbacks {
	return nil
}

type IPv6AddressHandler struct {
	dataplane operations.Dataplane
}

func NewIPv6AddressHandler(daemons *deps.ConfDeps) conf.Handler {
	return &IPv6AddressHandler{dataplane: daemons.Dataplane}
}

func (h *IPv6AddressHandler) Validate(ctx context.Context, hctx *conf.HandlerContext) error {
	addr, ok := hctx.NewValue.(string)
	if !ok {
		return fmt.Errorf("address must be a string")
	}

	ip, _, err := net.ParseCIDR(addr)
	if err != nil {
		return fmt.Errorf("invalid CIDR address: %w", err)
	}

	if ip.To4() != nil || !strings.Contains(addr, ":") {
		return fmt.Errorf("not an IPv6 address")
	}

	return nil
}

func (h *IPv6AddressHandler) Apply(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName := conf.ExtractInterfaceName(hctx.Path)
	addr := hctx.NewValue.(string)

	return h.dataplane.AddIPv6Address(ifName, addr)
}

func (h *IPv6AddressHandler) Rollback(ctx context.Context, hctx *conf.HandlerContext) error {
	ifName := conf.ExtractInterfaceName(hctx.Path)
	addr := hctx.NewValue.(string)

	return h.dataplane.DelIPv6Address(ifName, addr)
}

func (h *IPv6AddressHandler) PathPattern() paths.Path {
	return paths.InterfaceIPv6Address
}

func (h *IPv6AddressHandler) Dependencies() []paths.Path {
	return []paths.Path{paths.Interface}
}

func (h *IPv6AddressHandler) Callbacks() *conf.Callbacks {
	return nil
}
