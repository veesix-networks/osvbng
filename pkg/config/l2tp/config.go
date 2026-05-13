// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import "time"

// L2TPConfig is the top-level L2TPv2 configuration block. Control-plane
// policing for UDP/1701 punt is owned by the punt plugin config, not
// here.
type L2TPConfig struct {
	TunnelPools  map[string]*TunnelPool `json:"tunnel-pools,omitempty"  yaml:"tunnel-pools,omitempty"`
	Profiles     map[string]*Profile    `json:"profiles,omitempty"      yaml:"profiles,omitempty"`
	PeerPolicies map[string]*PeerPolicy `json:"peer-policies,omitempty" yaml:"peer-policies,omitempty"`
}

// TunnelPool is a named catalog of LNS endpoints used by the LAC as a
// static fallback when AAA returns no Tunnel-* attributes, and as the
// source of shared secrets when AAA returns endpoints but no
// Tunnel-Password.
type TunnelPool struct {
	LocalName string   `json:"local-name,omitempty" yaml:"local-name,omitempty"`
	LNS       []LNSRef `json:"lns,omitempty"        yaml:"lns,omitempty"`
}

// LNSRef is one LNS endpoint inside a tunnel-pool. Per-server VRF and
// source-ipv4 let one pool span multiple VRFs without forking groups.
type LNSRef struct {
	Name       string `json:"name"                  yaml:"name"`
	IPv4       string `json:"ipv4"                  yaml:"ipv4"`
	Secret     string `json:"secret,omitempty"      yaml:"secret,omitempty"`
	Preference uint16 `json:"preference,omitempty"  yaml:"preference,omitempty"`
	VRF        string `json:"vrf,omitempty"         yaml:"vrf,omitempty"`
	SourceIPv4 string `json:"source-ipv4,omitempty" yaml:"source-ipv4,omitempty"`
}

// Profile bundles timers, limits and policy. Referenced from a
// subscriber-group's l2tp block. Role-specific fields are honored only
// in that role; the others are ignored.
type Profile struct {
	SessionLimit      int               `json:"session-limit,omitempty"       yaml:"session-limit,omitempty"`
	HelloInterval     time.Duration     `json:"hello-interval,omitempty"      yaml:"hello-interval,omitempty"`
	ReceiveWindowSize int               `json:"receive-window-size,omitempty" yaml:"receive-window-size,omitempty"`
	DFBit             bool              `json:"df-bit,omitempty"              yaml:"df-bit,omitempty"`
	TunnelPool        string            `json:"tunnel-pool,omitempty"         yaml:"tunnel-pool,omitempty"`
	Retransmit        *RetransmitConfig `json:"retransmit,omitempty"          yaml:"retransmit,omitempty"`
	Denylist          *DenylistConfig   `json:"denylist,omitempty"            yaml:"denylist,omitempty"`

	// LNS-only.
	ChallengeRequired bool   `json:"challenge-required,omitempty" yaml:"challenge-required,omitempty"`
	ProxyLCPMode      string `json:"proxy-lcp-mode,omitempty"     yaml:"proxy-lcp-mode,omitempty"`

	// LAC-only.
	MaxAttemptsPerSubscriber int `json:"max-attempts-per-subscriber,omitempty" yaml:"max-attempts-per-subscriber,omitempty"`
}

type RetransmitConfig struct {
	MaxRetriesNotEstablished int           `json:"max-retries-not-established,omitempty" yaml:"max-retries-not-established,omitempty"`
	MaxRetriesEstablished    int           `json:"max-retries-established,omitempty"     yaml:"max-retries-established,omitempty"`
	InitialTimeout           time.Duration `json:"initial-timeout,omitempty"             yaml:"initial-timeout,omitempty"`
	MaxTimeout               time.Duration `json:"max-timeout,omitempty"                 yaml:"max-timeout,omitempty"`
}

type DenylistConfig struct {
	PeerTTL   time.Duration `json:"peer-ttl,omitempty"   yaml:"peer-ttl,omitempty"`
	TunnelTTL time.Duration `json:"tunnel-ttl,omitempty" yaml:"tunnel-ttl,omitempty"`
	Triggers  []string      `json:"triggers,omitempty"   yaml:"triggers,omitempty"`
}

// PeerPolicy authorizes an inbound LAC by hostname and binds it to a
// profile and a shared secret for Challenge-AVP auth. LNS-only.
type PeerPolicy struct {
	Hostname string `json:"hostname"          yaml:"hostname"`
	Secret   string `json:"secret,omitempty"  yaml:"secret,omitempty"`
	Profile  string `json:"profile,omitempty" yaml:"profile,omitempty"`
}

func (c *L2TPConfig) GetProfile(name string) *Profile {
	if c == nil {
		return nil
	}
	return c.Profiles[name]
}

func (c *L2TPConfig) GetTunnelPool(name string) *TunnelPool {
	if c == nil {
		return nil
	}
	return c.TunnelPools[name]
}

// GetPeerPolicyByHostname returns the peer policy whose Hostname
// matches the LAC Host Name AVP value, or nil if none match.
func (c *L2TPConfig) GetPeerPolicyByHostname(hostname string) *PeerPolicy {
	if c == nil {
		return nil
	}
	for _, p := range c.PeerPolicies {
		if p != nil && p.Hostname == hostname {
			return p
		}
	}
	return nil
}
