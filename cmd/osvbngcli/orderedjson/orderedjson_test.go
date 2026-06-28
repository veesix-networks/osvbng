// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package orderedjson

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Decode must return object keys in the exact order they appear in the input,
// every time. Looping guards against the map-iteration randomness that made
// the old map[string]any decode reshuffle columns between runs.
func TestDecodePreservesKeyOrder(t *testing.T) {
	raw := []byte(`{"username":"a@x","ip":"10.0.0.1","vlan":100,"circuit_id":"c1"}`)
	want := []string{"username", "ip", "vlan", "circuit_id"}

	for i := 0; i < 200; i++ {
		v, err := Decode(raw)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		obj, ok := v.(*Object)
		if !ok {
			t.Fatalf("want *Object, got %T", v)
		}
		if !reflect.DeepEqual(obj.Keys, want) {
			t.Fatalf("iteration %d: key order = %v, want %v", i, obj.Keys, want)
		}
	}
}

// Numbers decode as json.Number so large integer counters keep their exact
// textual form instead of being rounded through float64.
func TestDecodeNumbersExact(t *testing.T) {
	v, err := Decode([]byte(`{"rx_bytes":18446744073709551615}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	obj := v.(*Object)
	num, ok := obj.Vals["rx_bytes"].(json.Number)
	if !ok {
		t.Fatalf("want json.Number, got %T", obj.Vals["rx_bytes"])
	}
	if num.String() != "18446744073709551615" {
		t.Fatalf("number mangled: got %s", num.String())
	}
}

// MarshalJSON round-trips the original key order so `| json` output is ordered.
func TestMarshalJSONPreservesOrder(t *testing.T) {
	raw := []byte(`{"z":1,"a":2,"m":3}`)
	v, err := Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != `{"z":1,"a":2,"m":3}` {
		t.Fatalf("marshal order: got %s", out)
	}
}

func TestDecodeArrayAndNested(t *testing.T) {
	v, err := Decode([]byte(`[{"a":1},{"b":{"c":2}}]`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	arr, ok := v.([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("want 2-element []any, got %T len", v)
	}
	nested := arr[1].(*Object).Vals["b"].(*Object)
	if got, _ := nested.Get("c"); got.(json.Number).String() != "2" {
		t.Fatalf("nested decode wrong: %v", got)
	}
}

func TestDecodeEmptyIsNil(t *testing.T) {
	v, err := Decode(nil)
	if err != nil || v != nil {
		t.Fatalf("empty decode: v=%v err=%v", v, err)
	}
}
