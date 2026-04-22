// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package routing_policy

type RoutingPolicyConfig struct {
	PrefixSets         map[string]PrefixSet         `json:"prefix-sets,omitempty"          yaml:"prefix-sets,omitempty"`
	PrefixSetsV6       map[string]PrefixSet         `json:"prefix-sets-v6,omitempty"       yaml:"prefix-sets-v6,omitempty"`
	CommunitySets      map[string]CommunitySet      `json:"community-sets,omitempty"       yaml:"community-sets,omitempty"`
	ExtCommunitySets   map[string]ExtCommunitySet   `json:"ext-community-sets,omitempty"   yaml:"ext-community-sets,omitempty"`
	LargeCommunitySets map[string]LargeCommunitySet `json:"large-community-sets,omitempty" yaml:"large-community-sets,omitempty"`
	ASPathSets         map[string]ASPathSet         `json:"as-path-sets,omitempty"         yaml:"as-path-sets,omitempty"`
	RoutePolicies      map[string]RoutePolicy       `json:"route-policies,omitempty"       yaml:"route-policies,omitempty"`
}

type PrefixSet []PrefixSetEntry

type PrefixSetEntry struct {
	Sequence uint32 `json:"sequence,omitempty" yaml:"sequence,omitempty"`
	Prefix   string `json:"prefix"            yaml:"prefix"`
	Le       uint8  `json:"le,omitempty"      yaml:"le,omitempty"`
	Ge       uint8  `json:"ge,omitempty"      yaml:"ge,omitempty"`
	Action   string `json:"action"            yaml:"action"`
}

type CommunitySet []string

type ExtCommunitySet []string

type LargeCommunitySet []string

type ASPathSet []ASPathSetEntry

type ASPathSetEntry struct {
	Regex  string `json:"regex"  yaml:"regex"`
	Action string `json:"action" yaml:"action"`
}

type RoutePolicy []RoutePolicyEntry

type RoutePolicyEntry struct {
	Sequence uint32            `json:"sequence"              yaml:"sequence"`
	Action   string            `json:"action"                yaml:"action"`
	Match    *RoutePolicyMatch `json:"match,omitempty"       yaml:"match,omitempty"`
	Set      *RoutePolicySet   `json:"set,omitempty"         yaml:"set,omitempty"`
	Call     string            `json:"call,omitempty"        yaml:"call,omitempty"`
	OnMatch  string            `json:"on-match,omitempty"    yaml:"on-match,omitempty"`
}

type RoutePolicyMatch struct {
	PrefixSet         string `json:"prefix-set,omitempty"          yaml:"prefix-set,omitempty"`
	PrefixSetV6       string `json:"prefix-set-v6,omitempty"       yaml:"prefix-set-v6,omitempty"`
	CommunitySet      string `json:"community-set,omitempty"       yaml:"community-set,omitempty"`
	ExtCommunitySet   string `json:"ext-community-set,omitempty"   yaml:"ext-community-set,omitempty"`
	LargeCommunitySet string `json:"large-community-set,omitempty" yaml:"large-community-set,omitempty"`
	ASPathSet         string `json:"as-path-set,omitempty"         yaml:"as-path-set,omitempty"`
	Metric            uint32 `json:"metric,omitempty"              yaml:"metric,omitempty"`
	Tag               uint32 `json:"tag,omitempty"                 yaml:"tag,omitempty"`
}

type RoutePolicySet struct {
	LocalPreference        uint32 `json:"local-preference,omitempty"         yaml:"local-preference,omitempty"`
	Metric                 uint32 `json:"metric,omitempty"                   yaml:"metric,omitempty"`
	Weight                 uint32 `json:"weight,omitempty"                   yaml:"weight,omitempty"`
	Community              string `json:"community,omitempty"                yaml:"community,omitempty"`
	CommunityAdditive      bool   `json:"community-additive,omitempty"       yaml:"community-additive,omitempty"`
	CommunityDelete        string `json:"community-delete,omitempty"         yaml:"community-delete,omitempty"`
	LargeCommunity         string `json:"large-community,omitempty"          yaml:"large-community,omitempty"`
	LargeCommunityAdditive bool   `json:"large-community-additive,omitempty" yaml:"large-community-additive,omitempty"`
	ExtCommunityRT         string `json:"ext-community-rt,omitempty"         yaml:"ext-community-rt,omitempty"`
	ExtCommunitySoO        string `json:"ext-community-soo,omitempty"        yaml:"ext-community-soo,omitempty"`
	ASPathPrepend          string `json:"as-path-prepend,omitempty"          yaml:"as-path-prepend,omitempty"`
	Origin                 string `json:"origin,omitempty"                   yaml:"origin,omitempty"`
	Tag                    uint32 `json:"tag,omitempty"                      yaml:"tag,omitempty"`
	NextHopIPv4            string `json:"next-hop-ipv4,omitempty"            yaml:"next-hop-ipv4,omitempty"`
	NextHopIPv6            string `json:"next-hop-ipv6,omitempty"            yaml:"next-hop-ipv6,omitempty"`
}
