// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Values reaching the compact renderers come out of orderedjson, so numbers
// are json.Number, not float64. Every helper degrades to fmt.Sprintf("%v")
// on anything it cannot read rather than erroring: these functions feed
// display cells, and a weird value shown raw beats a render failure.

func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(n, 64)
		return f, err == nil
	}
	return 0, false
}

func asInt(v any) (int64, bool) {
	if n, ok := v.(json.Number); ok {
		i, err := n.Int64()
		if err == nil {
			return i, true
		}
	}
	f, ok := asFloat(v)
	return int64(f), ok
}

// siScale renders one decimal and trims a trailing ".0", so 50000000 is
// "50M" and 18200000 is "18.2M".
func siScale(f float64) string {
	units := []struct {
		div    float64
		suffix string
	}{
		{1e12, "T"},
		{1e9, "G"},
		{1e6, "M"},
		{1e3, "K"},
	}
	for _, u := range units {
		if f >= u.div {
			s := strconv.FormatFloat(f/u.div, 'f', 1, 64)
			s = strings.TrimSuffix(s, ".0")
			return s + u.suffix
		}
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// siCount humanizes an event or packet count: 1204 -> "1.2K".
func siCount(v any) string {
	f, ok := asFloat(v)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return siScale(f)
}

// siBytes humanizes a byte quantity in short form for cells whose header or
// label carries the unit: 25900000000 -> "25.9G".
func siBytes(v any) string {
	return siCount(v)
}

// siBytesUnit is the long, self-labelling form for prose lines with no
// header to lean on: 25900000000 -> "25.9 GB", 122 -> "122 B".
func siBytesUnit(v any) string {
	f, ok := asFloat(v)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	if f < 1e3 {
		return strconv.FormatFloat(f, 'f', -1, 64) + " B"
	}
	s := siScale(f)
	return s[:len(s)-1] + " " + s[len(s)-1:] + "B"
}

// siRateKbps humanizes a kbps rate as bits per second: 50000 -> "50M".
func siRateKbps(v any) string {
	f, ok := asFloat(v)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return siScale(f * 1000)
}

// pct renders used as a percentage of limit; a zero or unreadable limit is
// "0%" (an unlimited or unconfigured buffer is never "full").
func pct(used, limit any) string {
	u, okU := asFloat(used)
	l, okL := asFloat(limit)
	if !okU || !okL || l == 0 {
		return "0%"
	}
	return strconv.Itoa(int(u/l*100+0.5)) + "%"
}
