// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package svcgroup

import (
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config/qos"
)

type fakeApplier struct {
	schedulerCalls int
	schedulerRate  uint32
}

func (f *fakeApplier) ApplyIngressACL(uint32, string) error            { return nil }
func (f *fakeApplier) ApplyEgressACL(uint32, string) error             { return nil }
func (f *fakeApplier) RemoveIngressACL(uint32) error                   { return nil }
func (f *fakeApplier) RemoveEgressACL(uint32) error                    { return nil }
func (f *fakeApplier) EnableSourceVerify(uint32, bool) error           { return nil }
func (f *fakeApplier) DisableSourceVerify(uint32) error                { return nil }
func (f *fakeApplier) ApplyQoS(uint32, *qos.Policy, *qos.Policy) error { return nil }
func (f *fakeApplier) RemoveQoS(uint32) error                          { return nil }
func (f *fakeApplier) RemoveScheduler(uint32) error                    { return nil }

func (f *fakeApplier) ApplyScheduler(_ uint32, rateKbps uint32, _ *qos.SchedulerConfig) error {
	f.schedulerCalls++
	f.schedulerRate = rateKbps
	return nil
}

// The service-group DownloadRate is bps (docs/configuration/service-groups.md);
// ApplyScheduler takes kbps. A 10 Gbit/s AAA ad-hoc rate must arrive as
// 10,000,000 kbps, not be truncated through uint32 or passed through as bps.
func TestApplyToSessionDownloadRateBpsToKbps(t *testing.T) {
	sb := &fakeApplier{}
	sg := ServiceGroup{
		Name:         "test",
		QoSEgress:    "plan",
		DownloadRate: 10_000_000_000,
	}
	policies := map[string]*qos.Policy{
		"plan": {CIR: 100_000, Scheduler: &qos.SchedulerConfig{}},
	}

	if err := ApplyToSession(sb, 1, sg, policies); err != nil {
		t.Fatalf("ApplyToSession: %v", err)
	}
	if sb.schedulerCalls != 1 {
		t.Fatalf("ApplyScheduler calls = %d, want 1", sb.schedulerCalls)
	}
	if sb.schedulerRate != 10_000_000 {
		t.Errorf("scheduler rate = %d kbps, want 10000000", sb.schedulerRate)
	}
}

func TestApplyToSessionSchedulerRateFallsBackToCIR(t *testing.T) {
	sb := &fakeApplier{}
	sg := ServiceGroup{
		Name:      "test",
		QoSEgress: "plan",
	}
	policies := map[string]*qos.Policy{
		"plan": {CIR: 100_000, Scheduler: &qos.SchedulerConfig{}},
	}

	if err := ApplyToSession(sb, 1, sg, policies); err != nil {
		t.Fatalf("ApplyToSession: %v", err)
	}
	if sb.schedulerCalls != 1 {
		t.Fatalf("ApplyScheduler calls = %d, want 1", sb.schedulerCalls)
	}
	if sb.schedulerRate != 100_000 {
		t.Errorf("scheduler rate = %d kbps, want CIR passthrough 100000", sb.schedulerRate)
	}
}
