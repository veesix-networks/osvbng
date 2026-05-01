// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import "testing"

func TestParseMetricTag_Existing(t *testing.T) {
	tests := []struct {
		tag  string
		want metricSpec
	}{
		{"label", metricSpec{isLabel: true}},
		{"label=area", metricSpec{isLabel: true, labelName: "area"}},
		{"name=foo.bar,type=counter,help=hi", metricSpec{name: "foo.bar", kind: "counter", help: "hi"}},
		{"counter", metricSpec{kind: "counter"}},
		{"streaming_only", metricSpec{streamingOnly: true}},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			got := parseMetricTag(tt.tag)
			if got.isLabel != tt.want.isLabel || got.labelName != tt.want.labelName ||
				got.name != tt.want.name || got.kind != tt.want.kind ||
				got.help != tt.want.help || got.streamingOnly != tt.want.streamingOnly {
				t.Fatalf("parseMetricTag(%q) = %+v, want %+v", tt.tag, got, tt.want)
			}
		})
	}
}

func TestParseMetricTag_Flatten(t *testing.T) {
	got := parseMetricTag("flatten")
	if !got.flatten {
		t.Fatalf("expected flatten=true, got %+v", got)
	}
	if got.isLabel || got.mapKey || got.retainStale {
		t.Fatalf("flatten should not set other modes, got %+v", got)
	}
}

func TestParseMetricTag_MapKey(t *testing.T) {
	bare := parseMetricTag("map_key")
	if !bare.mapKey || !bare.isLabel {
		t.Fatalf("map_key should imply label, got %+v", bare)
	}

	mixed := parseMetricTag("label,map_key")
	if !mixed.mapKey || !mixed.isLabel {
		t.Fatalf("label,map_key should set both, got %+v", mixed)
	}

	named := parseMetricTag("label=area,map_key")
	if !named.mapKey || !named.isLabel || named.labelName != "area" {
		t.Fatalf("label=area,map_key should set labelName=area, got %+v", named)
	}
}

func TestParseMetricTag_RetainStale(t *testing.T) {
	tag := parseMetricTag("name=ha.srg.state,type=gauge,help=...,retain_stale")
	if !tag.retainStale {
		t.Fatalf("expected retainStale=true, got %+v", tag)
	}
	if tag.name != "ha.srg.state" || tag.kind != "gauge" {
		t.Fatalf("retain_stale should not interfere with other fields, got %+v", tag)
	}
}
