// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package routing

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/veesix-networks/osvbng/pkg/models/protocols/ldp"
)

func (c *Component) GetLDPNeighbors() ([]ldp.Neighbor, error) {
	output, err := c.execVtysh("-c", "show mpls ldp neighbor json")
	if err != nil {
		return nil, err
	}

	var wrapper struct {
		Neighbors []ldp.Neighbor `json:"neighbors"`
	}
	if err := json.Unmarshal(output, &wrapper); err != nil {
		return nil, fmt.Errorf("parse LDP neighbors: %w", err)
	}

	for i := range wrapper.Neighbors {
		wrapper.Neighbors[i].UpTimeSecs = parseLDPUpTime(wrapper.Neighbors[i].UpTime)
	}
	return wrapper.Neighbors, nil
}

func (c *Component) GetLDPNeighbor(ip string) (*ldp.Neighbor, error) {
	if err := validateLDPAddr(ip); err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show mpls ldp neighbor "+ip+" json")
	if err != nil {
		return nil, err
	}
	var wrapper map[string]ldp.Neighbor
	if err := json.Unmarshal(output, &wrapper); err != nil {
		return nil, fmt.Errorf("parse LDP neighbor %s: %w", ip, err)
	}
	n, ok := wrapper[ip]
	if !ok {
		return nil, nil
	}
	n.UpTimeSecs = parseLDPUpTime(n.UpTime)
	return &n, nil
}

func (c *Component) GetLDPNeighborDetail(ip string) (*ldp.NeighborDetail, error) {
	if err := validateLDPAddr(ip); err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show mpls ldp neighbor "+ip+" detail json")
	if err != nil {
		return nil, err
	}
	var wrapper map[string]ldp.NeighborDetail
	if err := json.Unmarshal(output, &wrapper); err != nil {
		return nil, fmt.Errorf("parse LDP neighbor %s detail: %w", ip, err)
	}
	d, ok := wrapper[ip]
	if !ok {
		return nil, nil
	}
	d.UpTimeSecs = parseLDPUpTime(d.UpTime)
	return &d, nil
}

func (c *Component) GetLDPCapabilities() ([]ldp.CapabilityTLV, error) {
	output, err := c.execVtysh("-c", "show mpls ldp capabilities json")
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Capabilities []ldp.CapabilityTLV `json:"capabilities"`
	}
	if err := json.Unmarshal(output, &wrapper); err != nil {
		return nil, fmt.Errorf("parse LDP capabilities: %w", err)
	}
	return wrapper.Capabilities, nil
}

func (c *Component) GetLDPNeighborsCapabilities() (map[string]ldp.NeighborCapabilities, error) {
	output, err := c.execVtysh("-c", "show mpls ldp neighbor capabilities json")
	if err != nil {
		return nil, err
	}
	out := map[string]ldp.NeighborCapabilities{}
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("parse LDP neighbors capabilities: %w", err)
	}
	return out, nil
}

func (c *Component) GetLDPNeighborCapabilities(ip string) (*ldp.NeighborCapabilities, error) {
	if err := validateLDPAddr(ip); err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", "show mpls ldp neighbor "+ip+" capabilities json")
	if err != nil {
		return nil, err
	}
	var wrapper map[string]ldp.NeighborCapabilities
	if err := json.Unmarshal(output, &wrapper); err != nil {
		return nil, fmt.Errorf("parse LDP neighbor %s capabilities: %w", ip, err)
	}
	caps, ok := wrapper[ip]
	if !ok {
		return nil, nil
	}
	return &caps, nil
}

func (c *Component) GetLDPIGPSync() (map[string]ldp.IGPSync, error) {
	output, err := c.execVtysh("-c", "show mpls ldp igp-sync json")
	if err != nil {
		return nil, err
	}
	out := map[string]ldp.IGPSync{}
	if err := json.Unmarshal(output, &out); err != nil {
		return nil, fmt.Errorf("parse LDP igp-sync: %w", err)
	}
	return out, nil
}

func (c *Component) GetLDPInterface(afi string) (map[string]ldp.Interface, error) {
	if err := validateLDPAFI(afi); err != nil {
		return nil, err
	}
	cmd := "show mpls ldp"
	if afi != "" {
		cmd += " " + afi
	}
	cmd += " interface json"
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return nil, err
	}
	var raw map[string]ldp.Interface
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("parse LDP interface: %w", err)
	}
	// FRR composite keys are "<iface>: <afi>"; split them so the typed
	// model emits clean labels under map_key on Interface.
	out := make(map[string]ldp.Interface, len(raw))
	for key, iface := range raw {
		name, af := splitLDPInterfaceKey(key)
		iface.Interface = name
		if iface.AddressFamily == "" {
			iface.AddressFamily = af
		}
		out[name] = iface
	}
	return out, nil
}

// BindingFilter collects the optional modifiers accepted by
// `show mpls ldp [<ipv4|ipv6>] binding ... [{neighbor X|local-label N|remote-label N}]`.
// At most one of Neighbor, LocalLabel, RemoteLabel may be set; the wrapper
// enforces that.
type BindingFilter struct {
	Neighbor    string
	LocalLabel  string
	RemoteLabel string
}

func (c *Component) GetLDPBindings(afi string, filter BindingFilter) ([]ldp.Binding, error) {
	cmd, err := bindingCommand(afi, "", false, filter, false)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Bindings []ldp.Binding `json:"bindings"`
	}
	if err := json.Unmarshal(output, &wrapper); err != nil {
		return nil, fmt.Errorf("parse LDP bindings: %w", err)
	}
	return wrapper.Bindings, nil
}

func (c *Component) GetLDPBindingByPrefix(afi, prefix string, longerPrefixes, detail bool, filter BindingFilter) (json.RawMessage, error) {
	if _, err := netip.ParsePrefix(prefix); err != nil {
		return nil, fmt.Errorf("invalid LDP binding prefix %q: %w", prefix, err)
	}
	cmd, err := bindingCommand(afi, prefix, longerPrefixes, filter, detail)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetLDPBindingsDetail(afi string) (json.RawMessage, error) {
	cmd, err := bindingCommand(afi, "", false, BindingFilter{}, true)
	if err != nil {
		return nil, err
	}
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (c *Component) GetLDPDiscovery(afi string) ([]ldp.Discovery, error) {
	if err := validateLDPAFI(afi); err != nil {
		return nil, err
	}
	cmd := "show mpls ldp"
	if afi != "" {
		cmd += " " + afi
	}
	cmd += " discovery json"
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return nil, err
	}
	var wrapper struct {
		Adjacencies []ldp.Discovery `json:"adjacencies"`
	}
	if err := json.Unmarshal(output, &wrapper); err != nil {
		return nil, fmt.Errorf("parse LDP discovery: %w", err)
	}
	return wrapper.Adjacencies, nil
}

func (c *Component) GetLDPDiscoveryDetail(afi string) (json.RawMessage, error) {
	if err := validateLDPAFI(afi); err != nil {
		return nil, err
	}
	cmd := "show mpls ldp"
	if afi != "" {
		cmd += " " + afi
	}
	cmd += " discovery detail json"
	output, err := c.execVtysh("-c", cmd)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func validateLDPAFI(afi string) error {
	if afi == "" || afi == "ipv4" || afi == "ipv6" {
		return nil
	}
	return fmt.Errorf("invalid LDP AFI %q", afi)
}

func validateLDPAddr(ip string) error {
	if _, err := netip.ParseAddr(ip); err != nil {
		return fmt.Errorf("invalid LDP neighbor address %q: %w", ip, err)
	}
	return nil
}

func validateLDPLabel(s string) error {
	if s == "" || s == "imp-null" {
		return nil
	}
	if _, err := strconv.ParseUint(s, 10, 32); err != nil {
		return fmt.Errorf("invalid LDP label %q: must be numeric or %q", s, "imp-null")
	}
	return nil
}

func bindingCommand(afi, prefix string, longerPrefixes bool, filter BindingFilter, detail bool) (string, error) {
	if err := validateLDPAFI(afi); err != nil {
		return "", err
	}
	count := 0
	if filter.Neighbor != "" {
		if err := validateLDPAddr(filter.Neighbor); err != nil {
			return "", err
		}
		count++
	}
	if filter.LocalLabel != "" {
		if err := validateLDPLabel(filter.LocalLabel); err != nil {
			return "", err
		}
		count++
	}
	if filter.RemoteLabel != "" {
		if err := validateLDPLabel(filter.RemoteLabel); err != nil {
			return "", err
		}
		count++
	}
	if count > 1 {
		return "", fmt.Errorf("LDP binding filter accepts at most one of neighbor / local-label / remote-label")
	}

	var b strings.Builder
	b.WriteString("show mpls ldp")
	if afi != "" {
		b.WriteByte(' ')
		b.WriteString(afi)
	}
	b.WriteString(" binding")
	if prefix != "" {
		b.WriteByte(' ')
		b.WriteString(prefix)
		if longerPrefixes {
			b.WriteString(" longer-prefixes")
		}
	}
	switch {
	case filter.Neighbor != "":
		b.WriteString(" neighbor ")
		b.WriteString(filter.Neighbor)
	case filter.LocalLabel != "":
		b.WriteString(" local-label ")
		b.WriteString(filter.LocalLabel)
	case filter.RemoteLabel != "":
		b.WriteString(" remote-label ")
		b.WriteString(filter.RemoteLabel)
	}
	if detail {
		b.WriteString(" detail")
	}
	b.WriteString(" json")
	return b.String(), nil
}

func splitLDPInterfaceKey(key string) (iface, afi string) {
	if i := strings.Index(key, ": "); i >= 0 {
		return key[:i], key[i+2:]
	}
	return key, ""
}

// parseLDPUpTime converts FRR's human-readable LDP uptime strings
// ("HH:MM:SS", "NdHHhMMm", "NwNd") to seconds. Unparseable input yields
// zero, matching the "absent" semantics for the gauge.
func parseLDPUpTime(s string) uint64 {
	if s == "" {
		return 0
	}

	// HH:MM:SS form.
	if strings.Count(s, ":") == 2 {
		parts := strings.Split(s, ":")
		h, _ := strconv.ParseUint(parts[0], 10, 64)
		m, _ := strconv.ParseUint(parts[1], 10, 64)
		sec, _ := strconv.ParseUint(parts[2], 10, 64)
		return h*3600 + m*60 + sec
	}

	// Compound form: "<num><unit>..." (e.g. "2d3h", "1w2d").
	var total uint64
	var num uint64
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			num = num*10 + uint64(ch-'0')
			continue
		}
		switch ch {
		case 'w':
			total += num * 7 * 24 * 3600
		case 'd':
			total += num * 24 * 3600
		case 'h':
			total += num * 3600
		case 'm':
			total += num * 60
		case 's':
			total += num
		}
		num = 0
	}
	return total
}
