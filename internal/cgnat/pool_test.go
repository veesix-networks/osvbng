// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"net"
	"sync"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/config/cgnat"
	"github.com/veesix-networks/osvbng/pkg/models"
)

func newTestPool(t *testing.T) *PoolManager {
	t.Helper()
	pm := NewPoolManager()
	cfg := &cgnat.Pool{
		Mode:                   "pba",
		BlockSize:              64,
		MaxBlocksPerSubscriber: 4,
		PortRange:              "1024-65535",
		AddressPooling:         "paired",
		OutsideAddresses:       []string{"100.64.0.0/30"},
	}
	if err := pm.ConfigurePool("p1", 1, cfg); err != nil {
		t.Fatalf("configure pool: %v", err)
	}
	return pm
}

func TestRestoreMappingIfAbsent_Idempotent(t *testing.T) {
	pm := newTestPool(t)
	m := &models.CGNATMapping{
		PoolName:       "p1",
		PoolID:         1,
		InsideIP:       net.ParseIP("10.0.0.1").To4(),
		InsideVRFID:    0,
		OutsideIP:      net.ParseIP("100.64.0.1").To4(),
		PortBlockStart: 1024,
		PortBlockEnd:   1087,
		SwIfIndex:      5,
	}

	if err := pm.RestoreMappingIfAbsent(m); err != nil {
		t.Fatalf("first restore: %v", err)
	}
	if err := pm.RestoreMappingIfAbsent(m); err != nil {
		t.Fatalf("second restore: %v", err)
	}
	if err := pm.RestoreMappingIfAbsent(m); err != nil {
		t.Fatalf("third restore: %v", err)
	}

	mappings := pm.GetMappings("p1", net.ParseIP("10.0.0.1").To4(), 0)
	if len(mappings) != 1 {
		t.Fatalf("expected 1 mapping after 3 RestoreMappingIfAbsent calls, got %d", len(mappings))
	}
}

func TestRestoreMapping_NotIdempotent_DoubleAppends(t *testing.T) {
	pm := newTestPool(t)
	m := &models.CGNATMapping{
		PoolName:       "p1",
		PoolID:         1,
		InsideIP:       net.ParseIP("10.0.0.1").To4(),
		OutsideIP:      net.ParseIP("100.64.0.1").To4(),
		PortBlockStart: 1024,
		PortBlockEnd:   1087,
		SwIfIndex:      5,
	}
	if err := pm.RestoreMapping(m); err != nil {
		t.Fatalf("first restore: %v", err)
	}
	if err := pm.RestoreMapping(m); err != nil {
		t.Fatalf("second restore: %v", err)
	}
	mappings := pm.GetMappings("p1", net.ParseIP("10.0.0.1").To4(), 0)
	if len(mappings) != 2 {
		t.Fatalf("RestoreMapping is expected to double-append (legacy behavior); got %d", len(mappings))
	}
}

func TestGetOrAllocate_ReturnsSameBlockForExisting(t *testing.T) {
	pm := newTestPool(t)
	insideIP := net.ParseIP("10.0.0.1").To4()

	m1, isNew, err := pm.GetOrAllocate("p1", insideIP, 0, 10)
	if err != nil {
		t.Fatalf("first GetOrAllocate: %v", err)
	}
	if !isNew {
		t.Fatalf("first GetOrAllocate should be new")
	}

	m2, isNew, err := pm.GetOrAllocate("p1", insideIP, 0, 10)
	if err != nil {
		t.Fatalf("second GetOrAllocate: %v", err)
	}
	if isNew {
		t.Fatalf("second GetOrAllocate should not be new")
	}
	if m1.PortBlockStart != m2.PortBlockStart || !m1.OutsideIP.Equal(m2.OutsideIP) {
		t.Fatalf("expected same block, got %v vs %v", m1, m2)
	}
}

func TestGetOrAllocate_ConcurrentSameSubscriber(t *testing.T) {
	pm := newTestPool(t)
	insideIP := net.ParseIP("10.0.0.1").To4()

	const N = 32
	var wg sync.WaitGroup
	results := make([]*models.CGNATMapping, N)
	newFlags := make([]bool, N)
	errs := make([]error, N)
	start := make(chan struct{})

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], newFlags[i], errs[i] = pm.GetOrAllocate("p1", insideIP, 0, 10)
		}(i)
	}
	close(start)
	wg.Wait()

	var newCount int
	for i := 0; i < N; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
		if newFlags[i] {
			newCount++
		}
	}
	if newCount != 1 {
		t.Fatalf("expected exactly 1 fresh allocation among %d concurrent callers, got %d", N, newCount)
	}

	first := results[0]
	for i := 1; i < N; i++ {
		if results[i].PortBlockStart != first.PortBlockStart || !results[i].OutsideIP.Equal(first.OutsideIP) {
			t.Fatalf("goroutine %d got a different block (%v) than the first (%v)", i, results[i], first)
		}
	}

	mappings := pm.GetMappings("p1", insideIP, 0)
	if len(mappings) != 1 {
		t.Fatalf("expected exactly 1 block allocated for subscriber after %d concurrent callers, got %d", N, len(mappings))
	}
}
