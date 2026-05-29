// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"sync"
	"testing"
)

func newActivationComponent() *Component {
	return &Component{
		sessionPoolMap: map[string]string{},
		activations:    map[string]struct{}{},
	}
}

func TestBeginActivation_SecondCallNoOps(t *testing.T) {
	c := newActivationComponent()

	ok, _ := c.beginActivation("s1")
	if !ok {
		t.Fatalf("first beginActivation should proceed")
	}

	ok2, _ := c.beginActivation("s1")
	if ok2 {
		t.Fatalf("second beginActivation while first in flight should NOT proceed")
	}
}

func TestBeginActivation_AlreadyCommittedNoOps(t *testing.T) {
	c := newActivationComponent()
	c.sessionPoolMap["s1"] = "pool-a"

	ok, _ := c.beginActivation("s1")
	if ok {
		t.Fatalf("beginActivation for already-committed session should NOT proceed")
	}
}

func TestBeginActivation_DoneAllowsRetry(t *testing.T) {
	c := newActivationComponent()
	ok, done := c.beginActivation("s1")
	if !ok {
		t.Fatalf("first beginActivation should proceed")
	}
	done()

	ok2, _ := c.beginActivation("s1")
	if !ok2 {
		t.Fatalf("after done() a second activation should proceed (e.g. retry after failure)")
	}
}

func TestBeginActivation_ConcurrentSameSession(t *testing.T) {
	const N = 64
	c := newActivationComponent()

	var wg sync.WaitGroup
	results := make([]bool, N)
	start := make(chan struct{})

	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			ok, done := c.beginActivation("s1")
			if ok {
				done()
			}
			results[i] = ok
		}(i)
	}
	close(start)
	wg.Wait()

	count := 0
	for _, ok := range results {
		if ok {
			count++
		}
	}
	if count == 0 {
		t.Fatalf("expected at least 1 successful beginActivation among %d, got 0", N)
	}
	if count == N {
		t.Fatalf("expected fewer than %d concurrent winners (most should be no-ops), got %d", N, count)
	}
}
