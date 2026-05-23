// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package routing

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"

	"github.com/veesix-networks/osvbng/pkg/models/protocols/zebra"
)

var validVRFNameRE = regexp.MustCompile(`^(all|[A-Za-z0-9_-]+)$`)

func vrfPrefix(vrf string) (string, error) {
	if vrf == "" {
		return "", nil
	}
	if !validVRFNameRE.MatchString(vrf) {
		return "", fmt.Errorf("invalid VRF name %q", vrf)
	}
	return "vrf " + vrf + " ", nil
}

func (c *Component) GetZebraRouteIPv4(vrf string) (json.RawMessage, error) {
	return c.zebraRoute("ip", vrf)
}

func (c *Component) GetZebraRouteIPv6(vrf string) (json.RawMessage, error) {
	return c.zebraRoute("ipv6", vrf)
}

func (c *Component) GetZebraRouteIPv4All() (json.RawMessage, error) {
	return c.zebraRouteAll("ip")
}

func (c *Component) GetZebraRouteIPv6All() (json.RawMessage, error) {
	return c.zebraRouteAll("ipv6")
}

func (c *Component) GetZebraRouteIPv4ByPrefix(vrf string, prefix netip.Prefix) (json.RawMessage, error) {
	if !prefix.Addr().Is4() {
		return nil, fmt.Errorf("expected IPv4 prefix, got %q", prefix)
	}
	return c.zebraRouteByPrefix("ip", vrf, prefix)
}

func (c *Component) GetZebraRouteIPv6ByPrefix(vrf string, prefix netip.Prefix) (json.RawMessage, error) {
	if prefix.Addr().Is4() {
		return nil, fmt.Errorf("expected IPv6 prefix, got %q", prefix)
	}
	return c.zebraRouteByPrefix("ipv6", vrf, prefix)
}

func (c *Component) GetZebraRouteSummary(vrf, afi string) (*zebra.RouteSummary, error) {
	if afi != "ipv4" && afi != "ipv6" {
		return nil, fmt.Errorf("invalid AFI %q", afi)
	}
	prefix, err := vrfPrefix(vrf)
	if err != nil {
		return nil, err
	}
	cmd := afiKeyword(afi) + " route " + prefix + "summary json"
	output, err := c.execVtysh("-c", "show "+cmd)
	if err != nil {
		return nil, err
	}
	var s zebra.RouteSummary
	if err := json.Unmarshal(output, &s); err != nil {
		return nil, fmt.Errorf("parse zebra %s route summary: %w", afi, err)
	}
	s.VRF = vrfDisplayName(vrf)
	return &s, nil
}

func (c *Component) GetZebraRouteSummaryAll(afi string) (*zebra.RouteSummaryAll, error) {
	if afi != "ipv4" && afi != "ipv6" {
		return nil, fmt.Errorf("invalid AFI %q", afi)
	}
	vrfs, err := c.GetVRFs()
	if err != nil {
		return nil, fmt.Errorf("list VRFs: %w", err)
	}
	out := &zebra.RouteSummaryAll{VRFs: make(map[string]zebra.RouteSummary, len(vrfs))}
	for _, v := range vrfs {
		vrfArg := v.Name
		if vrfArg == "default" {
			vrfArg = ""
		}
		s, err := c.GetZebraRouteSummary(vrfArg, afi)
		if err != nil {
			return nil, fmt.Errorf("zebra %s route summary for vrf %q: %w", afi, v.Name, err)
		}
		s.VRF = v.Name
		out.VRFs[v.Name] = *s
	}
	return out, nil
}

func (c *Component) GetZebraInterfaces(name string) (json.RawMessage, error) {
	cmd := "show interface"
	if name != "" {
		if !validInterfaceNameRE.MatchString(name) {
			return nil, fmt.Errorf("invalid interface name %q", name)
		}
		cmd += " " + name
	}
	cmd += " json"
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) zebraRoute(afiKW, vrf string) (json.RawMessage, error) {
	prefix, err := vrfPrefix(vrf)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show "+afiKeyword(afiKW)+" route "+prefix+"json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) zebraRouteAll(afiKW string) (json.RawMessage, error) {
	vrfs, err := c.GetVRFs()
	if err != nil {
		return nil, fmt.Errorf("list VRFs: %w", err)
	}
	merged := map[string]json.RawMessage{}
	for _, v := range vrfs {
		vrfArg := v.Name
		if vrfArg == "default" {
			vrfArg = ""
		}
		raw, err := c.zebraRoute(afiKW, vrfArg)
		if err != nil {
			return nil, fmt.Errorf("zebra %s route for vrf %q: %w", afiKW, v.Name, err)
		}
		var perVRF map[string]json.RawMessage
		if err := json.Unmarshal(raw, &perVRF); err != nil {
			return nil, fmt.Errorf("parse zebra %s route for vrf %q: %w", afiKW, v.Name, err)
		}
		for k, msg := range perVRF {
			merged[k] = msg
		}
	}
	return json.Marshal(merged)
}

func (c *Component) zebraRouteByPrefix(afiKW, vrf string, prefix netip.Prefix) (json.RawMessage, error) {
	prefixStr, err := vrfPrefix(vrf)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show "+afiKeyword(afiKW)+" route "+prefixStr+prefix.Masked().String()+" json")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func afiKeyword(afi string) string {
	switch afi {
	case "ip", "ipv4":
		return "ip"
	case "ipv6":
		return "ipv6"
	default:
		return afi
	}
}

func vrfDisplayName(vrf string) string {
	if vrf == "" {
		return "default"
	}
	return vrf
}

var validInterfaceNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
