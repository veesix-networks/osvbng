// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestConcurrentWritersNoLockError(t *testing.T) {
	t.Parallel()

	store, err := Open(filepath.Join(t.TempDir(), "opdb.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	const writers = 8
	const perWriter = 200

	var wg sync.WaitGroup
	errCh := make(chan error, writers*perWriter)

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(writer int) {
			defer wg.Done()
			ns := fmt.Sprintf("ns-%d", writer)
			for i := 0; i < perWriter; i++ {
				key := fmt.Sprintf("k-%d", i)
				val := []byte(fmt.Sprintf("writer=%d iter=%d", writer, i))
				if err := store.Put(context.Background(), ns, key, val); err != nil {
					errCh <- fmt.Errorf("writer %d iter %d: %w", writer, i, err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("write failed: %v", err)
	}

	for w := 0; w < writers; w++ {
		ns := fmt.Sprintf("ns-%d", w)
		count, err := store.Count(context.Background(), ns)
		if err != nil {
			t.Fatalf("count %s: %v", ns, err)
		}
		if count != perWriter {
			t.Errorf("namespace %s: expected %d rows, got %d", ns, perWriter, count)
		}
	}
}

func TestIsBusyClassifier(t *testing.T) {
	t.Parallel()

	if isBusy(nil) {
		t.Errorf("isBusy(nil) = true, want false")
	}
	if isBusy(fmt.Errorf("unrelated error")) {
		t.Errorf("isBusy(unrelated) = true, want false")
	}
	if !isBusy(fmt.Errorf("database is locked")) {
		t.Errorf("isBusy(database is locked) = false, want true")
	}
	if !isBusy(fmt.Errorf("database table is locked")) {
		t.Errorf("isBusy(database table is locked) = false, want true")
	}
}

func TestDSNEncodesPragmas(t *testing.T) {
	t.Parallel()

	dsn := buildDSN("/var/lib/osvbng/opdb.db")
	for _, want := range []string{
		"_journal_mode=WAL",
		"_synchronous=NORMAL",
		"_busy_timeout=5000",
		"_cache_size=-64000",
	} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN missing %q: %s", want, dsn)
		}
	}
}
