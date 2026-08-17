// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config/system"
)

func TestAutoLayoutTiers(t *testing.T) {
	cases := []struct {
		total   int
		workers string
		cp      string
	}{
		{1, "", ""},
		{2, "1", ""},
		{3, "1", "2"},
		{4, "1-2", "3"},
		{5, "1-3", "4"},
		{7, "1-5", "6"},
		{8, "1-5", "6-7"},
		{16, "1-13", "14-15"},
		{64, "1-61", "62-63"},
	}
	for _, c := range cases {
		if got := autoWorkerCores(c.total); got != c.workers {
			t.Errorf("autoWorkerCores(%d) = %q, want %q", c.total, got, c.workers)
		}
		if got := autoCPCores(c.total); got != c.cp {
			t.Errorf("autoCPCores(%d) = %q, want %q", c.total, got, c.cp)
		}
	}
}

// Main core 0, workers and CP must partition the remaining cores with no
// overlap at every host size.
func TestAutoLayoutDisjoint(t *testing.T) {
	for total := 3; total <= 96; total++ {
		workers, err := parseCoreSet(autoWorkerCores(total))
		if err != nil {
			t.Fatalf("total=%d: bad worker set: %v", total, err)
		}
		cp, err := parseCoreSet(autoCPCores(total))
		if err != nil {
			t.Fatalf("total=%d: bad cp set: %v", total, err)
		}
		if len(cp) == 0 {
			t.Fatalf("total=%d: no cp cores", total)
		}
		for core := range cp {
			if workers[core] {
				t.Fatalf("total=%d: core %d in both worker and cp sets", total, core)
			}
			if core == 0 || core >= total {
				t.Fatalf("total=%d: cp core %d out of range", total, core)
			}
		}
		for core := range workers {
			if core == 0 || core >= total {
				t.Fatalf("total=%d: worker core %d out of range", total, core)
			}
		}
	}
}

func TestResolveCPCoresPriority(t *testing.T) {
	cfg := &Config{}

	t.Setenv("OSVBNG_CP_CORES", "4-5")
	r := ResolveCPULayout(cfg)
	if r.CPCores != "4-5" {
		t.Errorf("env override: got %q, want 4-5", r.CPCores)
	}

	cfg.Dataplane = system.DataplaneConfig{CPCores: "2-3,34-35"}
	r = ResolveCPULayout(cfg)
	if r.CPCores != "2-3,34-35" {
		t.Errorf("yaml over env: got %q, want 2-3,34-35", r.CPCores)
	}
	if n := r.CPCoreCount(); n != 4 {
		t.Errorf("CPCoreCount() = %d, want 4", n)
	}
}

func TestResolveAutoSentinel(t *testing.T) {
	cfg := &Config{}

	t.Setenv("OSVBNG_DP_WORKER_CORES", "auto")
	t.Setenv("OSVBNG_CP_CORES", "auto")
	r := ResolveCPULayout(cfg)
	total := DetectAvailableCores()
	if r.WorkerCores != autoWorkerCores(total) {
		t.Errorf("worker auto sentinel: got %q, want auto layout %q", r.WorkerCores, autoWorkerCores(total))
	}
	if r.CPCores != autoCPCores(total) {
		t.Errorf("cp auto sentinel: got %q, want auto layout %q", r.CPCores, autoCPCores(total))
	}

	t.Setenv("OSVBNG_DP_WORKER_CORES", "7-8")
	t.Setenv("OSVBNG_CP_CORES", "9")
	r = ResolveCPULayout(cfg)
	if r.WorkerCores != "7-8" || r.CPCores != "9" {
		t.Errorf("slot values: got %q/%q, want 7-8/9", r.WorkerCores, r.CPCores)
	}
}
