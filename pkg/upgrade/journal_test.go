// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestJournalReadMissingReturnsErrNotExist(t *testing.T) {
	dir := t.TempDir()
	j := NewJournal(filepath.Join(dir, "absent.json"))

	_, err := j.Read()
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Read on missing journal: err = %v, want fs.ErrNotExist", err)
	}
}

func TestJournalWriteThenReadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	j := NewJournal(filepath.Join(dir, "state.json"))

	original := &JournalState{
		From:      "0.13.0",
		To:        "0.13.1",
		Tarball:   "/tmp/osvbng-v0.13.1.tar.gz",
		StartedAt: time.Date(2026, 6, 15, 8, 34, 0, 0, time.UTC),
		Phase:     "snapshot_done",
		CompletedArtifacts: []string{
			"/usr/local/bin/osvbngd",
		},
		PendingArtifacts: []string{
			"/usr/local/bin/osvbngcli",
		},
	}

	if err := j.Write(original); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if original.Version != JournalVersion {
		t.Fatalf("Write did not fill Version: got %d, want %d", original.Version, JournalVersion)
	}
	if original.UpdatedAt.IsZero() {
		t.Fatalf("Write did not stamp UpdatedAt")
	}

	loaded, err := j.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if loaded.From != "0.13.0" || loaded.To != "0.13.1" {
		t.Fatalf("From/To not preserved: %q -> %q", loaded.From, loaded.To)
	}
	if loaded.Phase != "snapshot_done" {
		t.Fatalf("Phase not preserved: %q", loaded.Phase)
	}
	if len(loaded.CompletedArtifacts) != 1 || loaded.CompletedArtifacts[0] != "/usr/local/bin/osvbngd" {
		t.Fatalf("CompletedArtifacts not preserved: %v", loaded.CompletedArtifacts)
	}
	if len(loaded.PendingArtifacts) != 1 || loaded.PendingArtifacts[0] != "/usr/local/bin/osvbngcli" {
		t.Fatalf("PendingArtifacts not preserved: %v", loaded.PendingArtifacts)
	}
	if loaded.Version != JournalVersion {
		t.Fatalf("Version not preserved: %d", loaded.Version)
	}
}

func TestJournalSetPhaseUpdatesOnlyPhase(t *testing.T) {
	dir := t.TempDir()
	j := NewJournal(filepath.Join(dir, "state.json"))

	original := &JournalState{
		From:      "0.13.0",
		To:        "0.13.1",
		StartedAt: time.Date(2026, 6, 15, 8, 34, 0, 0, time.UTC),
		Phase:     "snapshot_done",
		CompletedArtifacts: []string{
			"/usr/local/bin/osvbngd",
		},
	}
	if err := j.Write(original); err != nil {
		t.Fatalf("Write: %v", err)
	}
	initialUpdatedAt := original.UpdatedAt
	beforeMtime := mustMtime(t, j.Path())
	time.Sleep(10 * time.Millisecond)

	if err := j.SetPhase("swapping:/usr/local/bin/osvbngcli"); err != nil {
		t.Fatalf("SetPhase: %v", err)
	}

	loaded, err := j.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if loaded.Phase != "swapping:/usr/local/bin/osvbngcli" {
		t.Fatalf("Phase not updated: %q", loaded.Phase)
	}
	if loaded.From != "0.13.0" || loaded.To != "0.13.1" {
		t.Fatalf("SetPhase clobbered From/To: %q -> %q", loaded.From, loaded.To)
	}
	if len(loaded.CompletedArtifacts) != 1 {
		t.Fatalf("SetPhase clobbered CompletedArtifacts: %v", loaded.CompletedArtifacts)
	}
	if !loaded.UpdatedAt.After(initialUpdatedAt) {
		t.Fatalf("UpdatedAt did not advance: %v vs %v", loaded.UpdatedAt, initialUpdatedAt)
	}
	afterMtime := mustMtime(t, j.Path())
	if !afterMtime.After(beforeMtime) {
		t.Fatalf("file mtime did not advance after SetPhase: %v vs %v", afterMtime, beforeMtime)
	}
}

func TestJournalSetPhaseRefusesMissing(t *testing.T) {
	dir := t.TempDir()
	j := NewJournal(filepath.Join(dir, "absent.json"))

	err := j.SetPhase("anything")
	if err == nil {
		t.Fatal("SetPhase on missing journal: err = nil, want error")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("SetPhase on missing journal: err = %v, want wrapped fs.ErrNotExist", err)
	}
}

func TestJournalClearIdempotent(t *testing.T) {
	dir := t.TempDir()
	j := NewJournal(filepath.Join(dir, "state.json"))

	if err := j.Clear(); err != nil {
		t.Fatalf("Clear on absent journal: %v", err)
	}

	if err := j.Write(&JournalState{From: "a", To: "b", Phase: "completed"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := j.Clear(); err != nil {
		t.Fatalf("Clear on present journal: %v", err)
	}
	if _, err := os.Stat(j.Path()); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Clear left the file behind: stat err = %v", err)
	}

	if err := j.Clear(); err != nil {
		t.Fatalf("Clear on freshly-cleared journal: %v", err)
	}
}

func TestJournalWriteIsAtomic(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "state.json")
	j := NewJournal(target)

	first := &JournalState{From: "0.13.0", To: "0.13.1", Phase: "snapshot_done"}
	if err := j.Write(first); err != nil {
		t.Fatalf("first Write: %v", err)
	}

	originalBytes, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read original: %v", err)
	}

	// Concurrent writers + readers. The reader must never see an empty
	// or partial file; either the original payload or one of the new
	// payloads is acceptable.
	var (
		wg            sync.WaitGroup
		readerObserved = 0
	)
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			data, err := os.ReadFile(target)
			if err != nil {
				continue
			}
			var s JournalState
			if err := json.Unmarshal(data, &s); err != nil {
				t.Errorf("reader saw invalid JSON: %v (bytes=%q)", err, string(data))
				return
			}
			readerObserved++
		}
	}()

	for i := 0; i < 50; i++ {
		_ = j.Write(&JournalState{
			From:    "0.13.0",
			To:      "0.13.1",
			Phase:   "swapping",
			Message: "iteration",
		})
	}

	close(stop)
	wg.Wait()

	if readerObserved == 0 {
		t.Fatal("concurrent reader observed zero successful decodes")
	}

	// Final file is still readable.
	if _, err := j.Read(); err != nil {
		t.Fatalf("final Read: %v", err)
	}
	_ = originalBytes
}

func mustMtime(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.ModTime()
}
