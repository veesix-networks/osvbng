// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/veesix-networks/osvbng/cmd/osvbngcli/orderedjson"
)

// Compact renderers, keyed by the show envelope's dotted path. They exist
// because the generic table gives these views one column per JSON field
// (~30 for schedulers) and JSON-blobs the nested tins array. Only the cli
// output format consults them - | json and | yaml carry the full payload
// untouched - and a renderer that does not recognise the payload shape
// returns false so the generic formatter still stands behind it.
var compactRenderers = map[string]func(any) (string, bool){
	"qos.scheduler": renderSchedulerTable,
	"qos.aggregate": renderAggregateTree,
}

func renderCompact(path string, payload any) (string, bool) {
	r, ok := compactRenderers[path]
	if !ok {
		return "", false
	}
	return r(payload)
}

// str returns the field as a display string, dash for empty or missing.
func str(obj *orderedjson.Object, key string) string {
	v, ok := obj.Get(key)
	if !ok || v == nil {
		return "-"
	}
	s := fmt.Sprintf("%v", v)
	if s == "" {
		return "-"
	}
	return s
}

func field(obj *orderedjson.Object, key string) any {
	v, _ := obj.Get(key)
	return v
}

func boolField(obj *orderedjson.Object, key string) bool {
	v, _ := obj.Get(key)
	b, _ := v.(bool)
	return b
}

// tinModeAbbrev compresses the tin mode for the MODE column; the expansion
// legend lives in the command's help text.
func tinModeAbbrev(mode string) string {
	m := strings.ToLower(strings.TrimPrefix(mode, "OSVBNG_CAKE_TIN_MODE_"))
	switch m {
	case "besteffort":
		return "be"
	case "diffserv3":
		return "ds3"
	case "diffserv4":
		return "ds4"
	case "diffserv8":
		return "ds8"
	}
	return mode
}

func objectRows(payload any) ([]*orderedjson.Object, bool) {
	list, ok := payload.([]any)
	if !ok {
		return nil, false
	}
	rows := make([]*orderedjson.Object, 0, len(list))
	for _, e := range list {
		obj, ok := e.(*orderedjson.Object)
		if !ok {
			return nil, false
		}
		rows = append(rows, obj)
	}
	return rows, true
}

func alignedTable(headers []string, rows [][]string) string {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	var sb strings.Builder
	for i, h := range headers {
		fmt.Fprintf(&sb, "%-*s  ", widths[i], h)
	}
	sb.WriteString("\n")
	for _, w := range widths {
		sb.WriteString(strings.Repeat("-", w) + "  ")
	}
	sb.WriteString("\n")
	for _, row := range rows {
		for i, cell := range row {
			fmt.Fprintf(&sb, "%-*s  ", widths[i], cell)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// renderSchedulerTable is one line per subscriber scheduler. Everything
// dropped here (per-tin stats, enqueue counters, deficit, config echoes)
// lives in `show qos scheduler detail` / `session`, and in | json.
func renderSchedulerTable(payload any) (string, bool) {
	rows, ok := objectRows(payload)
	if !ok {
		return "", false
	}
	if len(rows) == 0 {
		return "No data\n", true
	}

	headers := []string{"SW_IF", "INTERFACE", "SESSION", "RATE", "MODE", "W(EFF)",
		"PARENT", "ST", "TX PKTS/BYTES", "DROP", "Q", "BUF", "BLK D/P"}

	cells := make([][]string, 0, len(rows))
	for _, obj := range rows {
		if _, ok := obj.Get("sw_if_index"); !ok {
			return "", false
		}

		// Effective weight is rate-derived (bytes/sec scaled), so it is a
		// large number worth humanizing; zero means a v1-only dataplane
		// that has no DRR state to report.
		wEff := "-"
		if eff, ok := asInt(field(obj, "effective_weight")); ok && eff != 0 {
			wEff = fmt.Sprintf("%s(%s)", str(obj, "weight"), siCount(field(obj, "effective_weight")))
		}

		parent := "(none)"
		if boolField(obj, "has_parent") {
			at := str(obj, "parent_interface")
			if at == "-" {
				at = str(obj, "parent_sw_if_index")
			}
			if str(obj, "parent_level") == "svlan" {
				parent = fmt.Sprintf("sv%s@%s", str(obj, "parent_svlan_id"), at)
			} else {
				parent = "port@" + at
			}
		}

		st := "-"
		if boolField(obj, "drr_active") {
			st = "A"
		}

		cells = append(cells, []string{
			str(obj, "sw_if_index"),
			str(obj, "interface"),
			str(obj, "session_id"),
			siRateKbps(field(obj, "rate_kbps")),
			tinModeAbbrev(str(obj, "tin_mode")),
			wEff,
			parent,
			st,
			siCount(field(obj, "dequeued_pkts")) + "/" + siBytes(field(obj, "dequeued_bytes")),
			siCount(field(obj, "dropped_pkts")),
			str(obj, "queued_buffers"),
			pct(field(obj, "buffer_usage"), field(obj, "buffer_limit")),
			siCount(field(obj, "drr_blocked")) + "/" + siCount(field(obj, "parent_blocked")),
		})
	}

	return alignedTable(headers, cells), true
}

// renderAggregateTree groups the flat aggregate rows into the hierarchy
// they describe: each port aggregate with its S-VLAN aggregates beneath it.
// Purely a render-side grouping - the API payload stays row-per-aggregate.
func renderAggregateTree(payload any) (string, bool) {
	rows, ok := objectRows(payload)
	if !ok {
		return "", false
	}
	if len(rows) == 0 {
		return "No data\n", true
	}

	type group struct {
		name   string
		port   *orderedjson.Object
		svlans []*orderedjson.Object
	}
	byName := map[string]*group{}
	var names []string

	for _, obj := range rows {
		if _, ok := obj.Get("sw_if_index"); !ok {
			return "", false
		}
		name := str(obj, "interface")
		if name == "-" {
			name = "if" + str(obj, "sw_if_index")
		}
		g, ok := byName[name]
		if !ok {
			g = &group{name: name}
			byName[name] = g
			names = append(names, name)
		}
		switch str(obj, "level") {
		case "port":
			g.port = obj
		case "svlan":
			g.svlans = append(g.svlans, obj)
		default:
			return "", false
		}
	}
	sort.Strings(names)

	var sb strings.Builder
	for gi, name := range names {
		if gi > 0 {
			sb.WriteString("\n")
		}
		g := byName[name]
		sort.Slice(g.svlans, func(i, j int) bool {
			a, _ := asInt(field(g.svlans[i], "svlan_id"))
			b, _ := asInt(field(g.svlans[j], "svlan_id"))
			return a < b
		})

		if g.port != nil {
			p := g.port
			fmt.Fprintf(&sb, "%s  port  %s  ·  %s active / W %s  ·  buf %s/%s (%s)\n",
				g.name,
				siRateKbps(field(p, "rate_kbps")),
				str(p, "active_children"),
				siCount(field(p, "active_weight")),
				siBytes(field(p, "buffer_usage")),
				siBytes(field(p, "buffer_limit")),
				pct(field(p, "buffer_usage"), field(p, "buffer_limit")))

			cont := "        "
			if len(g.svlans) > 0 {
				cont = "│       "
			}
			fmt.Fprintf(&sb, "%sshaped %s pkts / %s   backpressure %s   parent-blk %s\n",
				cont,
				siCount(field(p, "shaped_pkts")),
				siBytesUnit(field(p, "shaped_bytes")),
				siCount(field(p, "backpressure")),
				siCount(field(p, "parent_blocked")))
			if len(g.svlans) == 0 {
				sb.WriteString("        (no svlan aggregates)\n")
			}
		}

		for i, sv := range g.svlans {
			label := "svlan " + str(sv, "svlan_id")
			if str(sv, "svlan_id_end") != str(sv, "svlan_id") {
				label += "-" + str(sv, "svlan_id_end")
			}

			branch, cont := "├─ ", "│       "
			if i == len(g.svlans)-1 {
				branch, cont = "└─ ", "        "
			}
			if g.port == nil {
				// The port row was filtered out (e.g. --level svlan):
				// render flat under the interface name, no tree glyphs.
				branch, cont = g.name+"  ", "        "
			}

			fmt.Fprintf(&sb, "%s%s  %s  w%s (eff %s)   %s active / W %s   buf %s/%s (%s)\n",
				branch, label,
				siRateKbps(field(sv, "rate_kbps")),
				str(sv, "weight"),
				siCount(field(sv, "effective_weight")),
				str(sv, "active_children"),
				siCount(field(sv, "active_weight")),
				siBytes(field(sv, "buffer_usage")),
				siBytes(field(sv, "buffer_limit")),
				pct(field(sv, "buffer_usage"), field(sv, "buffer_limit")))
			fmt.Fprintf(&sb, "%sshaped %s pkts / %s   backpressure %s   blk drr %s par %s\n",
				cont,
				siCount(field(sv, "shaped_pkts")),
				siBytesUnit(field(sv, "shaped_bytes")),
				siCount(field(sv, "backpressure")),
				siCount(field(sv, "drr_blocked")),
				siCount(field(sv, "parent_blocked")))
		}
	}

	return sb.String(), true
}
