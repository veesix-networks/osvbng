package qos

import (
	"fmt"
	"strconv"
	"strings"
)

// Aggregate levels, matching osvbng_cake_agg_level in the dataplane API.
const (
	AggregateLevelPort  uint8 = 0
	AggregateLevelSVLAN uint8 = 1
)

// Weight bounds, matching CAKE_WEIGHT_MIN/MAX in the dataplane. The upper
// bound is what the dataplane's quantum arithmetic is sized against, so it is
// enforced here too rather than left to be rejected over the wire.
const (
	WeightMin uint32 = 1
	WeightMax uint32 = 256
)

// Burst bounds in milliseconds, matching CAKE_AGG_BURST_NS_MIN/MAX.
const (
	BurstMsMin uint32 = 10
	BurstMsMax uint32 = 150
)

// MaxSVLANID is one past the largest 802.1Q tag the dataplane's per-port map
// holds.
const MaxSVLANID = 4095

// Aggregate is one tier of the shaping hierarchy: a port, or an S-VLAN
// beneath one.
//
// Both are configured against the port's interface. An entry with no svlans
// shapes the port itself; an entry with svlans shapes the traffic of those
// tags, and competes with its sibling S-VLANs for the port's rate in
// proportion to Rate x Weight.
//
// Oversubscription across S-VLANs is expected: the sum of their rates
// normally exceeds the port, which is the whole reason the tier exists. What
// is refused is a single S-VLAN shaped above the port it hangs off, which
// cannot mean anything.
type Aggregate struct {
	Name      string   `json:"name,omitempty"      yaml:"-"`
	Interface string   `json:"interface"           yaml:"interface"`
	SVLANs    []string `json:"svlans,omitempty"    yaml:"svlans,omitempty"`
	Rate      uint32   `json:"rate"                yaml:"rate"`
	Weight    uint32   `json:"weight,omitempty"    yaml:"weight,omitempty"`
	BurstMs   uint32   `json:"burst-ms,omitempty"  yaml:"burst-ms,omitempty"`

	// BufferLimit in bytes; zero lets the dataplane derive it from the rate.
	BufferLimit uint32 `json:"buffer-limit,omitempty" yaml:"buffer-limit,omitempty"`
}

// SVLANRange is one inclusive tag range.
type SVLANRange struct {
	First uint16
	Last  uint16
}

// Level reports which tier this entry configures.
func (a *Aggregate) Level() uint8 {
	if len(a.SVLANs) == 0 {
		return AggregateLevelPort
	}
	return AggregateLevelSVLAN
}

// ParseSVLANs expands the configured range strings.
//
// Each entry is a single tag or one hyphenated range. Comma syntax inside an
// entry is rejected rather than silently accepted, because the list is how
// disjoint sets are expressed and accepting both spellings invites configs
// that mean different things in different places.
func (a *Aggregate) ParseSVLANs() ([]SVLANRange, error) {
	out := make([]SVLANRange, 0, len(a.SVLANs))

	for _, entry := range a.SVLANs {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, fmt.Errorf("empty svlan entry")
		}
		if strings.Contains(entry, ",") {
			return nil, fmt.Errorf("svlan entry %q: use separate list entries, not commas", entry)
		}

		first, last, err := parseSVLANEntry(entry)
		if err != nil {
			return nil, err
		}
		out = append(out, SVLANRange{First: first, Last: last})
	}

	return out, nil
}

func parseSVLANEntry(entry string) (uint16, uint16, error) {
	parts := strings.SplitN(entry, "-", 2)

	first, err := parseTag(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("svlan entry %q: %w", entry, err)
	}

	last := first
	if len(parts) == 2 {
		if last, err = parseTag(parts[1]); err != nil {
			return 0, 0, fmt.Errorf("svlan entry %q: %w", entry, err)
		}
	}

	if last < first {
		return 0, 0, fmt.Errorf("svlan entry %q: range is descending", entry)
	}

	return first, last, nil
}

func parseTag(s string) (uint16, error) {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 10, 16)
	if err != nil {
		return 0, fmt.Errorf("not a vlan tag: %q", s)
	}
	if v > MaxSVLANID {
		return 0, fmt.Errorf("vlan tag %d above %d", v, MaxSVLANID)
	}
	return uint16(v), nil
}

// Validate checks one entry in isolation. Constraints that need the whole set
// - disjoint tags, a child not exceeding its port - are in ValidateAggregates.
func (a *Aggregate) Validate() error {
	if a.Interface == "" {
		return fmt.Errorf("qos-aggregate %q: interface is required", a.Name)
	}
	if a.Rate == 0 {
		return fmt.Errorf("qos-aggregate %q: rate is required", a.Name)
	}
	if a.Weight != 0 && (a.Weight < WeightMin || a.Weight > WeightMax) {
		return fmt.Errorf("qos-aggregate %q: weight %d outside %d-%d",
			a.Name, a.Weight, WeightMin, WeightMax)
	}
	if a.BurstMs != 0 && (a.BurstMs < BurstMsMin || a.BurstMs > BurstMsMax) {
		return fmt.Errorf("qos-aggregate %q: burst-ms %d outside %d-%d",
			a.Name, a.BurstMs, BurstMsMin, BurstMsMax)
	}
	if _, err := a.ParseSVLANs(); err != nil {
		return fmt.Errorf("qos-aggregate %q: %w", a.Name, err)
	}
	return nil
}

// ValidateAggregates checks the set: every S-VLAN has a port to hang off, tag
// sets are disjoint per port, and no S-VLAN is shaped above its port.
func ValidateAggregates(aggs map[string]*Aggregate) error {
	ports := make(map[string]*Aggregate)

	for name, a := range aggs {
		a.Name = name
		if err := a.Validate(); err != nil {
			return err
		}
		if a.Level() != AggregateLevelPort {
			continue
		}
		if prev, dup := ports[a.Interface]; dup {
			return fmt.Errorf("qos-aggregate %q: interface %s already has a port aggregate (%q)",
				name, a.Interface, prev.Name)
		}
		ports[a.Interface] = a
	}

	claimed := make(map[string]map[uint16]string)

	for name, a := range aggs {
		if a.Level() != AggregateLevelSVLAN {
			continue
		}

		port, ok := ports[a.Interface]
		if !ok {
			return fmt.Errorf("qos-aggregate %q: no port aggregate configured for interface %s",
				name, a.Interface)
		}
		if a.Rate > port.Rate {
			return fmt.Errorf("qos-aggregate %q: rate %d exceeds its port %q rate %d",
				name, a.Rate, port.Name, port.Rate)
		}

		ranges, err := a.ParseSVLANs()
		if err != nil {
			return err
		}
		if claimed[a.Interface] == nil {
			claimed[a.Interface] = make(map[uint16]string)
		}
		for _, r := range ranges {
			for tag := uint32(r.First); tag <= uint32(r.Last); tag++ {
				if owner, taken := claimed[a.Interface][uint16(tag)]; taken {
					return fmt.Errorf("qos-aggregate %q: svlan %d on %s already claimed by %q",
						name, tag, a.Interface, owner)
				}
				claimed[a.Interface][uint16(tag)] = name
			}
		}
	}

	return nil
}
