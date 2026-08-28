// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
)

// Validate is the only gate between a config file and the component that
// programs it (pkg/config/loader.go). Anything it lets through reaches
// ConfigurePool inside Component.Start, which the orchestrator runs with no
// recover: a panic there is a boot loop that survives every restart until
// somebody edits the file by hand.
//
// The spellings below are the ones the dataplane implements, taken from the
// plugin's .api enums and the pool table in docs/configuration/cgnat.md.
// blacklist-mode and exhaustion-behavior are deliberately not checked: neither
// has a consumer or a stated value set, so a spelling check here would be
// inventing one. Refusing them outright belongs with the rest of the knobs
// that parse and change nothing (osvbng#486).
var (
	poolModes       = []string{"pba", "deterministic"}
	addressPoolings = []string{"paired", "arbitrary"}
	filterings      = []string{"endpoint-independent", "endpoint-dependent"}
)

// The allocator holds one entry per outside address, each with its own block
// bitmap, so a pool's memory is linear in the size of its prefixes: roughly
// 6 MB for a /16, 1.7 GB for a /8, which does not survive boot. /16 is also
// about eight million port blocks at the default block size, orders of
// magnitude past what one BNG serves, so nothing an operator would really
// configure is being refused here. Widening it means teaching the allocator
// to hold ranges rather than addresses.
const minOutsidePrefixLen = 16

func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	if c.LegacyOutsideInterface != nil {
		return fmt.Errorf("cgnat: outside_interface (string) has been replaced by per-pool outside_interfaces (list); move it under each pool block")
	}
	if len(c.LegacyOutsideInterfaces) > 0 {
		return fmt.Errorf("cgnat: outside_interfaces at the top level has moved into each pool block; move 'outside_interfaces: [...]' under 'cgnat.pools.<name>.outside_interfaces' for each pool")
	}
	if c.Reconcile != nil && c.Reconcile.OnDivergence != "" &&
		c.Reconcile.OnDivergence != "reconcile" &&
		c.Reconcile.OnDivergence != "fail" {
		return fmt.Errorf("cgnat: reconcile.on_divergence must be \"reconcile\" or \"fail\", got %q", c.Reconcile.OnDivergence)
	}

	// Pools are a map. Sort so a config with two faults always reports the
	// same one and an operator's second run says what the first did.
	names := make([]string, 0, len(c.Pools))
	for name := range c.Pools {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		pool := c.Pools[name]
		if pool == nil {
			continue
		}
		if err := pool.validate(name); err != nil {
			return err
		}
	}

	return validateNoPoolOverlap(c, names)
}

func (p *Pool) validate(name string) error {
	if len(p.OutsideInterfaces) == 0 {
		return fmt.Errorf("cgnat: pool %q: outside_interfaces is required", name)
	}
	seen := make(map[string]struct{}, len(p.OutsideInterfaces))
	for i, entry := range p.OutsideInterfaces {
		if entry == "" {
			return fmt.Errorf("cgnat: pool %q: outside_interfaces[%d]: empty interface name", name, i)
		}
		if _, dup := seen[entry]; dup {
			return fmt.Errorf("cgnat: pool %q: outside_interfaces: duplicate entry %q", name, entry)
		}
		seen[entry] = struct{}{}
	}

	if err := validateEnum(name, "mode", p.Mode, poolModes); err != nil {
		return err
	}
	if err := validateEnum(name, "address-pooling", p.AddressPooling, addressPoolings); err != nil {
		return err
	}
	if err := validateEnum(name, "filtering", p.Filtering, filterings); err != nil {
		return err
	}

	if err := p.validatePortsAndBlocks(name); err != nil {
		return err
	}

	for i, prefix := range p.InsidePrefixes {
		if _, err := ParseInsidePrefix(prefix.Prefix); err != nil {
			// FindPoolForIP skips a prefix it cannot parse and returns no
			// pool, so the subscriber comes up untranslated with nothing
			// logged. Refuse it here instead.
			return fmt.Errorf("cgnat: pool %q: inside-prefixes[%d]: %w", name, i, err)
		}
	}

	for i, addr := range p.OutsideAddresses {
		prefix, err := ParseOutsideAddress(addr)
		if err != nil {
			return fmt.Errorf("cgnat: pool %q: outside-addresses[%d]: %w", name, i, err)
		}
		if ones, _ := prefix.Mask.Size(); ones < minOutsidePrefixLen {
			return fmt.Errorf("cgnat: pool %q: outside-addresses[%d]: %s is wider than /%d and would expand to %d allocator entries; split the pool if the outside space is really this large",
				name, i, addr, minOutsidePrefixLen, uint64(1)<<uint(32-ones))
		}
	}

	for i, addr := range p.ExcludedAddresses {
		if _, err := ParseExcludedAddress(addr); err != nil {
			return fmt.Errorf("cgnat: pool %q: excluded-addresses[%d]: %w", name, i, err)
		}
	}

	return nil
}

func validateEnum(pool, field, value string, allowed []string) error {
	if value == "" {
		return nil
	}
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("cgnat: pool %q: %s must be one of %s, got %q",
		pool, field, strings.Join(allowed, ", "), value)
}

// validatePortsAndBlocks refuses every input that makes GetBlockSize resolve
// to zero, which ConfigurePool then divides the port range by.
func (p *Pool) validatePortsAndBlocks(name string) error {
	start, end := uint16(defaultPortRangeStart), uint16(defaultPortRangeEnd)
	if p.PortRange != "" {
		var err error
		start, end, err = parsePortRangeStrict(p.PortRange)
		if err != nil {
			return fmt.Errorf("cgnat: pool %q: port-range: %w", name, err)
		}
	}
	usable := uint32(end) - uint32(start) + 1

	switch {
	case p.BlockSize > 0:
		if uint32(p.BlockSize) > usable {
			return fmt.Errorf("cgnat: pool %q: block-size %d exceeds the %d ports in port-range %d-%d",
				name, p.BlockSize, usable, start, end)
		}
	case p.SubscriberRatio > 0:
		if uint32(p.SubscriberRatio) > usable {
			return fmt.Errorf("cgnat: pool %q: subscriber-ratio %d exceeds the %d ports in port-range %d-%d, so the block size computes to 0",
				name, p.SubscriberRatio, usable, start, end)
		}
	default:
		if uint32(defaultBlockSize) > usable {
			return fmt.Errorf("cgnat: pool %q: the default block-size %d exceeds the %d ports in port-range %d-%d; set block-size explicitly",
				name, defaultBlockSize, usable, start, end)
		}
	}
	return nil
}

// validateNoPoolOverlap refuses two pools claiming the same address.
// FindPoolForIP walks the pool map, whose iteration order is random, and
// returns the first prefix that contains the address, so an overlap picks a
// different pool per call. Overlap within one pool is not ambiguous and is
// left alone: a pool carries the same prefix in several VRFs in the
// wholesale shape.
func validateNoPoolOverlap(c *Config, names []string) error {
	type claim struct {
		pool   string
		vrf    string
		text   string
		prefix *net.IPNet
	}
	var inside, outside []claim

	for _, name := range names {
		pool := c.Pools[name]
		if pool == nil {
			continue
		}

		for _, entry := range pool.InsidePrefixes {
			prefix, err := ParseInsidePrefix(entry.Prefix)
			if err != nil {
				continue
			}
			for _, prev := range inside {
				if prev.pool == name || !vrfsCanCollide(prev.vrf, entry.VRF) {
					continue
				}
				if prefixesOverlap(prev.prefix, prefix) {
					return fmt.Errorf("cgnat: pools %q and %q both claim inside prefix %s%s; a subscriber address in both selects a pool at random",
						prev.pool, name, entry.Prefix, vrfNote(entry.VRF))
				}
			}
			inside = append(inside, claim{pool: name, vrf: entry.VRF, text: entry.Prefix, prefix: prefix})
		}

		for _, addr := range pool.OutsideAddresses {
			prefix, err := ParseOutsideAddress(addr)
			if err != nil {
				continue
			}
			for _, prev := range outside {
				if prev.pool == name {
					continue
				}
				if prefixesOverlap(prev.prefix, prefix) {
					return fmt.Errorf("cgnat: pools %q and %q both claim outside address %s (also %s); one address cannot be allocated by two pools",
						prev.pool, name, addr, prev.text)
				}
			}
			outside = append(outside, claim{pool: name, text: addr, prefix: prefix})
		}
	}
	return nil
}

// An empty VRF matches any VRF at classification time, so it collides with
// every named one.
func vrfsCanCollide(a, b string) bool {
	return a == b || a == "" || b == ""
}

func vrfNote(vrf string) string {
	if vrf == "" {
		return ""
	}
	return fmt.Sprintf(" in vrf %q", vrf)
}

func prefixesOverlap(a, b *net.IPNet) bool {
	return a.Contains(b.IP) || b.Contains(a.IP)
}

// ParseInsidePrefix accepts an IPv4 CIDR.
func ParseInsidePrefix(s string) (*net.IPNet, error) {
	_, prefix, err := net.ParseCIDR(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("%q is not an IPv4 CIDR", s)
	}
	if prefix.IP.To4() == nil {
		return nil, fmt.Errorf("%q is not IPv4", s)
	}
	return prefix, nil
}

// ParseOutsideAddress accepts a bare IPv4 address or an IPv4 CIDR, and
// returns the prefix it covers. A bare address is a /32.
func ParseOutsideAddress(s string) (*net.IPNet, error) {
	t := strings.TrimSpace(s)
	if ip := net.ParseIP(t); ip != nil {
		ip4 := ip.To4()
		if ip4 == nil {
			return nil, fmt.Errorf("%q is not IPv4", s)
		}
		return &net.IPNet{IP: ip4, Mask: net.CIDRMask(32, 32)}, nil
	}
	_, prefix, err := net.ParseCIDR(t)
	if err != nil {
		return nil, fmt.Errorf("%q is not an IPv4 address or CIDR", s)
	}
	if prefix.IP.To4() == nil {
		return nil, fmt.Errorf("%q is not IPv4", s)
	}
	return prefix, nil
}

// ParseExcludedAddress accepts a bare IPv4 address or a /32, and returns the
// address. The allocator matches an exclusion against net.IP.String(), so
// both spellings have to normalise to the same text or the entry never
// matches anything and the address is handed out regardless.
func ParseExcludedAddress(s string) (net.IP, error) {
	t := strings.TrimSpace(s)
	if ip := net.ParseIP(t); ip != nil {
		ip4 := ip.To4()
		if ip4 == nil {
			return nil, fmt.Errorf("%q is not IPv4", s)
		}
		return ip4, nil
	}
	ip, prefix, err := net.ParseCIDR(t)
	if err != nil {
		return nil, fmt.Errorf("%q is not an IPv4 address", s)
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return nil, fmt.Errorf("%q is not IPv4", s)
	}
	if ones, _ := prefix.Mask.Size(); ones != 32 {
		return nil, fmt.Errorf("%q must be a single address or a /32", s)
	}
	return ip4, nil
}

func parsePortRangeStrict(s string) (uint16, uint16, error) {
	startText, endText, ok := strings.Cut(strings.TrimSpace(s), "-")
	if !ok {
		return 0, 0, fmt.Errorf("expected \"start-end\", got %q", s)
	}
	start, err := parsePort(startText)
	if err != nil {
		return 0, 0, fmt.Errorf("start: %w", err)
	}
	end, err := parsePort(endText)
	if err != nil {
		return 0, 0, fmt.Errorf("end: %w", err)
	}
	if start == 0 {
		return 0, 0, fmt.Errorf("start must be 1 or higher")
	}
	if end < start {
		return 0, 0, fmt.Errorf("end %d is below start %d", end, start)
	}
	return start, end, nil
}

func parsePort(s string) (uint16, error) {
	t := strings.TrimSpace(s)
	v, err := strconv.ParseUint(t, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("%q is not a port number", t)
	}
	return uint16(v), nil
}
