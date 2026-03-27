// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

type ResolvedCPU struct {
	MainCore    int
	WorkerCores string
	CPCores     string
	TotalCores  int
}

// ResolveCPULayout determines the CPU core layout based on available cores.
//
// Layout table:
//
//	Cores | Main (VPP+FRR) | Workers    | Go CP
//	------+----------------+------------+-----------
//	1     | 0              | -          | shared (0)
//	2     | 0              | 1          | shared (0)
//	3     | 0              | 1          | 2
//	4     | 0              | 1-2        | 3
//	5-7   | 0              | 1-(N-2)    | N-1
//	8+    | 0              | 1-(N-3)    | (N-2)-(N-1)
//
// Env var overrides (OSVBNG_DP_MAIN_CORE, OSVBNG_DP_WORKER_CORES, OSVBNG_CP_CORES)
// and YAML config (dataplane.main-core, dataplane.workers) take priority over
// auto-detection.
func ResolveCPULayout(cfg *Config) *ResolvedCPU {
	total := DetectAvailableCores()

	resolved := &ResolvedCPU{
		MainCore:   0,
		TotalCores: total,
	}

	// Priority for main core: YAML config > env var > default (0)
	if cfg.Dataplane.MainCore != nil {
		resolved.MainCore = *cfg.Dataplane.MainCore
	} else if v := os.Getenv("OSVBNG_DP_MAIN_CORE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			resolved.MainCore = n
		}
	}

	// Priority for workers: YAML config > env var > auto-layout
	if cfg.Dataplane.Workers != "" {
		resolved.WorkerCores = cfg.Dataplane.Workers
	} else if v := os.Getenv("OSVBNG_DP_WORKER_CORES"); v != "" {
		resolved.WorkerCores = v
	} else {
		resolved.WorkerCores = autoWorkerCores(total)
	}

	// Priority for CP cores: env var > auto-layout
	if v := os.Getenv("OSVBNG_CP_CORES"); v != "" {
		resolved.CPCores = v
	} else {
		resolved.CPCores = autoCPCores(total)
	}

	return resolved
}

func autoWorkerCores(total int) string {
	if total > 1 {
		return "1"
	}
	return ""
}

func autoCPCores(total int) string {
	if total >= 3 {
		return "2"
	}
	return ""
}

// WriteEnvFile writes the resolved layout as shell-evaluable variables.
func (r *ResolvedCPU) WriteEnvFile(path string) error {
	content := fmt.Sprintf("OSVBNG_RESOLVED_MAIN_CORE=%d\nOSVBNG_RESOLVED_WORKER_CORES=%s\nOSVBNG_RESOLVED_CP_CORES=%s\nOSVBNG_RESOLVED_TOTAL_CORES=%d\n",
		r.MainCore, r.WorkerCores, r.CPCores, r.TotalCores)
	return os.WriteFile(path, []byte(content), 0644)
}

// CPCoreCount returns the number of control plane cores in the resolved layout.
func (r *ResolvedCPU) CPCoreCount() int {
	cores, err := parseCoreSet(r.CPCores)
	if err != nil {
		return 1
	}
	if len(cores) == 0 {
		return 1
	}
	return len(cores)
}

func ValidateCPU(r *ResolvedCPU) error {
	mainSet := map[int]bool{r.MainCore: true}

	workerSet, err := parseCoreSet(r.WorkerCores)
	if err != nil {
		return fmt.Errorf("invalid worker cores %q: %w", r.WorkerCores, err)
	}

	cpSet, err := parseCoreSet(r.CPCores)
	if err != nil {
		return fmt.Errorf("invalid control-plane cores %q: %w", r.CPCores, err)
	}

	for core := range workerSet {
		if mainSet[core] {
			return fmt.Errorf("dataplane worker core %d overlaps with main core %d — check dataplane.workers in your config", core, r.MainCore)
		}
		if core >= r.TotalCores {
			return fmt.Errorf("dataplane worker core %d requested but only %d cores available (0-%d) — remove dataplane.workers from your config to use auto-detection, or reduce the worker range to fit within the available cores",
				core, r.TotalCores, r.TotalCores-1)
		}
	}

	for core := range cpSet {
		if workerSet[core] {
			return fmt.Errorf("control-plane core %d overlaps with dataplane worker core — check OSVBNG_CP_CORES and dataplane.workers in your config", core)
		}
		if core >= r.TotalCores {
			return fmt.Errorf("control-plane core %d requested but only %d cores available (0-%d) — reduce OSVBNG_CP_CORES or increase the available CPU allocation",
				core, r.TotalCores, r.TotalCores-1)
		}
	}

	return nil
}

// DetectAvailableCores returns the number of CPUs available to this process,
// respecting cgroup limits (Docker, Kubernetes, systemd).
func DetectAvailableCores() int {
	if n := cgroupCPUCount(); n > 0 {
		return n
	}
	return runtime.NumCPU()
}

func cgroupCPUCount() int {
	// cgroups v2: /sys/fs/cgroup/cpu.max contains "quota period"
	if data, err := os.ReadFile("/sys/fs/cgroup/cpu.max"); err == nil {
		parts := strings.Fields(strings.TrimSpace(string(data)))
		if len(parts) == 2 && parts[0] != "max" {
			quota, err1 := strconv.Atoi(parts[0])
			period, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil && period > 0 {
				cores := quota / period
				if cores < 1 {
					cores = 1
				}
				return cores
			}
		}
	}

	// cgroups v2: cpuset.cpus.effective
	if data, err := os.ReadFile("/sys/fs/cgroup/cpuset.cpus.effective"); err == nil {
		if n := countCoresFromSet(strings.TrimSpace(string(data))); n > 0 {
			return n
		}
	}

	// cgroups v1: cpu quota
	quota := readIntFile("/sys/fs/cgroup/cpu/cpu.cfs_quota_us")
	period := readIntFile("/sys/fs/cgroup/cpu/cpu.cfs_period_us")
	if quota > 0 && period > 0 {
		cores := quota / period
		if cores < 1 {
			cores = 1
		}
		return cores
	}

	// cgroups v1: cpuset
	if data, err := os.ReadFile("/sys/fs/cgroup/cpuset/cpuset.cpus"); err == nil {
		if n := countCoresFromSet(strings.TrimSpace(string(data))); n > 0 {
			return n
		}
	}

	return 0
}

func countCoresFromSet(cpuSet string) int {
	if cpuSet == "" {
		return 0
	}
	cores, err := parseCoreSet(cpuSet)
	if err != nil {
		return 0
	}
	return len(cores)
}

func parseCoreSet(s string) (map[int]bool, error) {
	cores := make(map[int]bool)
	if s == "" {
		return cores, nil
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			lo, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid core range %q", part)
			}
			hi, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid core range %q", part)
			}
			for i := lo; i <= hi; i++ {
				cores[i] = true
			}
		} else {
			n, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid core %q", part)
			}
			cores[n] = true
		}
	}
	return cores, nil
}

func readIntFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return n
}
