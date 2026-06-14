// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/config/subscriber"
	"github.com/veesix-networks/osvbng/pkg/telemetry"
)

var ipoeDropFamilyV4 = telemetry.MustRegisterCounter(telemetry.CounterOpts{
	Name:   "ipoe.dhcpv4.dropped_family_disabled",
	Help:   "DHCPv4 packets dropped at ingress because the subscriber group has no ipv4-profile bound.",
	Labels: []string{"group"},
})

var ipoeDropFamilyV6 = telemetry.MustRegisterCounter(telemetry.CounterOpts{
	Name:   "ipoe.dhcpv6.dropped_family_disabled",
	Help:   "DHCPv6 packets dropped at ingress because the subscriber group has no ipv6-profile bound.",
	Labels: []string{"group"},
})

var ipoeDropForeignServer = telemetry.MustRegisterCounter(telemetry.CounterOpts{
	Name:   "ipoe.dhcpv4.dropped_foreign_server",
	Help:   "Server-sourced DHCPv4 messages (OFFER/ACK/NAK) dropped because osvbng is the authoritative DHCP server on the access VLAN; indicates a foreign or rogue DHCP server.",
	Labels: []string{"group", "type"},
})

var ndDropFamily = telemetry.MustRegisterCounter(telemetry.CounterOpts{
	Name:   "ipoe.nd.dropped_family_disabled",
	Help:   "IPv6 ND packets (RS/NS) dropped because the subscriber group has no ipv6-profile bound.",
	Labels: []string{"group", "type"},
})

var aaaAttrDropFamily = telemetry.MustRegisterCounter(telemetry.CounterOpts{
	Name:   "aaa.attr.dropped_family_disabled",
	Help:   "AAA Access-Accept attributes stripped because the subscriber group does not enable that address family.",
	Labels: []string{"group", "family"},
})

func groupV4Enabled(g *subscriber.SubscriberGroup) bool { return g != nil && g.IPv4Profile != "" }
func groupV6Enabled(g *subscriber.SubscriberGroup) bool { return g != nil && g.IPv6Profile != "" }

var v4FamilyAttrs = []string{
	aaa.AttrIPv4Address, aaa.AttrIPv4Netmask, aaa.AttrIPv4Gateway,
	aaa.AttrDNSPrimary, aaa.AttrDNSSecondary, aaa.AttrPool,
}

var v6FamilyAttrs = []string{
	aaa.AttrIPv6Address, aaa.AttrIPv6Prefix, aaa.AttrIPv6WANPrefix,
	aaa.AttrIPv6DNSPrimary, aaa.AttrIPv6DNSSecondary, aaa.AttrIANAPool, aaa.AttrPDPool,
}

func countFamilyAttrs(attrs map[string]interface{}, keys []string) int {
	n := 0
	for _, k := range keys {
		if _, ok := attrs[k]; ok {
			n++
		}
	}
	return n
}
