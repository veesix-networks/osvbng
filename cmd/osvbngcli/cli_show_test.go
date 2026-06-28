// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/veesix-networks/osvbng/cmd/osvbngcli/orderedjson"
)

// Exercises the actual OpenAPI show path (cli.go CommandShow): the northbound
// envelope is decoded, its Data is order-preserved, then formatted. This is the
// path the running CLI uses; the columns must be stable and in struct order.
func TestShowEnvelopePathStableColumns(t *testing.T) {
	// Exactly what plugins/northbound/api returns for `show system threads`
	// (json.Marshal of ShowResponse{Data: []system.Thread}).
	body := []byte(`{"path":"system/threads","data":[` +
		`{"ID":0,"Name":"vpp_main","Type":"","ProcessID":57,"CPUID":0,"CPUCore":0,"CPUSocket":0},` +
		`{"ID":1,"Name":"vpp_wk_0","Type":"workers","ProcessID":66,"CPUID":1,"CPUCore":1,"CPUSocket":0}]}`)

	want := []string{"ID", "Name", "Type", "ProcessID", "CPUID", "CPUCore", "CPUSocket"}

	var first string
	for i := 0; i < 200; i++ {
		var env showResponseEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("envelope decode: %v", err)
		}
		payload, err := orderedjson.Decode(env.Data)
		if err != nil {
			t.Fatalf("data decode: %v", err)
		}
		out, err := NewGenericFormatter().Format(payload, FormatCLI)
		if err != nil {
			t.Fatalf("format: %v", err)
		}
		if i == 0 {
			first = out
			header := strings.Fields(strings.SplitN(out, "\n", 2)[0])
			if !reflect.DeepEqual(header, want) {
				t.Fatalf("column order = %v, want %v", header, want)
			}
			continue
		}
		if out != first {
			t.Fatalf("run %d differs from run 0:\n--- run 0 ---\n%s\n--- run %d ---\n%s", i, first, i, out)
		}
	}
}
