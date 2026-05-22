// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf6

import (
	"context"
	"fmt"
	"strconv"

	"github.com/veesix-networks/osvbng/internal/routing"
	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
)

func init() {
	show.RegisterFactory(NewOSPF6InterfacePrefixHandler)
}

type OSPF6InterfacePrefixHandler struct {
	routing *routing.Component
}

func NewOSPF6InterfacePrefixHandler(deps *deps.ShowDeps) show.ShowHandler {
	return &OSPF6InterfacePrefixHandler{routing: deps.Routing}
}

func (h *OSPF6InterfacePrefixHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.routing == nil {
		return nil, fmt.Errorf("routing component not available")
	}
	detail, _ := strconv.ParseBool(req.Options["detail"])
	match, _ := strconv.ParseBool(req.Options["match"])
	return h.routing.GetOSPF6InterfacePrefix(
		req.Options["vrf"],
		req.Options["interface"],
		req.Options["prefix"],
		detail, match,
	)
}

func (h *OSPF6InterfacePrefixHandler) PathPattern() paths.Path {
	return paths.ProtocolsOSPF6InterfacePrefix
}

func (h *OSPF6InterfacePrefixHandler) Dependencies() []paths.Path {
	return nil
}

func (h *OSPF6InterfacePrefixHandler) Summary() string {
	return "Show OSPFv3 interface prefixes"
}

func (h *OSPF6InterfacePrefixHandler) Description() string {
	return "Display the OSPFv3 prefixes attached to each interface; response forwarded as-is from FRR."
}

type OSPF6InterfacePrefixOptions struct {
	VRF       string `query:"vrf" description:"VRF name; empty means the default routing table"`
	Interface string `query:"interface" description:"Restrict to one interface name"`
	Prefix    string `query:"prefix" description:"Filter by IPv6 prefix"`
	Detail    bool   `query:"detail" description:"Return the detail variant of the response"`
	Match     bool   `query:"match" description:"Treat prefix as a longest-match query"`
}

func (h *OSPF6InterfacePrefixHandler) OptionsType() interface{} {
	return &OSPF6InterfacePrefixOptions{}
}
