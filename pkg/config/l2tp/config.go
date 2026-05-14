// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"strings"
	"time"
)

// PPPFramingHDLC and PPPFramingCompressed select how a session is
// expected to frame PPP packets on the wire. HDLC = Address+Control
// (0xff 0x03) prefix present on every PPP frame (default; matches
// pppd-based LACs out of the box). Compressed = ACFC in effect, no
// prefix.
const (
	PPPFramingHDLC       = "hdlc"
	PPPFramingCompressed = "compressed"
)

// PPPFraming is an embeddable per-entity override for the PPP framing
// mode on a session. Resolution is most-specific-wins via Merge.
type PPPFraming struct {
	Framing string `json:"ppp-framing,omitempty" yaml:"ppp-framing,omitempty"`
}

// Merge returns a copy of `p` with non-empty fields from `override`
// taking precedence. Used to fold a profile-level default with a
// per-server / per-peer-policy override.
func (p PPPFraming) Merge(override PPPFraming) PPPFraming {
	if override.Framing != "" {
		p.Framing = override.Framing
	}
	return p
}

// PPPHdrSkip returns the byte count the dataplane should advance past
// before reading the PPP protocol field. 2 for HDLC framing (default),
// 0 for compressed (ACFC). Unknown / unset values fall back to HDLC.
func (p PPPFraming) PPPHdrSkip() uint8 {
	switch strings.ToLower(p.Framing) {
	case PPPFramingCompressed:
		return 0
	default:
		return 2
	}
}

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

// LNSRef is one LNS endpoint inside a tunnel-pool. Per-server VRF,
// source-ipv4, and PPP framing override let one pool span multiple
// upstream LNSes with mixed behavior.
type LNSRef struct {
	Name       string `json:"name"                  yaml:"name"`
	IPv4       string `json:"ipv4"                  yaml:"ipv4"`
	Secret     string `json:"secret,omitempty"      yaml:"secret,omitempty"`
	Preference uint16 `json:"preference,omitempty"  yaml:"preference,omitempty"`
	VRF        string `json:"vrf,omitempty"         yaml:"vrf,omitempty"`
	SourceIPv4 string `json:"source-ipv4,omitempty" yaml:"source-ipv4,omitempty"`
	PPPFraming `yaml:",inline"`
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
	PPPFraming        `yaml:",inline"`

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
	Hostname   string `json:"hostname"          yaml:"hostname"`
	Secret     string `json:"secret,omitempty"  yaml:"secret,omitempty"`
	Profile    string `json:"profile,omitempty" yaml:"profile,omitempty"`
	PPPFraming `yaml:",inline"`
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

// ResolvePPPFramingLNS picks the framing for an LNS session: a peer-
// policy override wins, otherwise the profile default, otherwise
// HDLC. Used at session-create on the LNS side.
func ResolvePPPFramingLNS(profile *Profile, policy *PeerPolicy) PPPFraming {
	var f PPPFraming
	if profile != nil {
		f = f.Merge(profile.PPPFraming)
	}
	if policy != nil {
		f = f.Merge(policy.PPPFraming)
	}
	if f.Framing == "" {
		f.Framing = PPPFramingHDLC
	}
	return f
}

// ResolvePPPFramingLAC picks the framing for a LAC session: the
// upstream LNS server entry wins, otherwise the profile default,
// otherwise HDLC. Used at LAC session bring-up.
func ResolvePPPFramingLAC(profile *Profile, lns *LNSRef) PPPFraming {
	var f PPPFraming
	if profile != nil {
		f = f.Merge(profile.PPPFraming)
	}
	if lns != nil {
		f = f.Merge(lns.PPPFraming)
	}
	if f.Framing == "" {
		f.Framing = PPPFramingHDLC
	}
	return f
}
