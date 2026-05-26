// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"context"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestSignalGuardFirstSignalCancelsContext(t *testing.T) {
	guard := NewSignalGuard()
	ctx, stop := guard.Install(context.Background())
	defer stop()

	// Send SIGINT to ourselves; ctx must cancel within a short window.
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("kill self: %v", err)
	}

	select {
	case <-ctx.Done():
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("ctx was not cancelled within 2s of first signal")
	}
}

func TestSignalGuardSecondSignalForcesExit(t *testing.T) {
	var exitCode atomic.Int32
	exitCode.Store(-1)
	exitCh := make(chan struct{}, 1)
	guard := NewSignalGuard().WithExit(func(code int) {
		exitCode.Store(int32(code))
		select {
		case exitCh <- struct{}{}:
		default:
		}
	})

	ctx, stop := guard.Install(context.Background())
	defer stop()

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("first kill: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("ctx not cancelled after first signal")
	}

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("second kill: %v", err)
	}

	select {
	case <-exitCh:
	case <-time.After(2 * time.Second):
		t.Fatal("exit was not invoked after second signal")
	}
	if got := exitCode.Load(); got != 2 {
		t.Fatalf("exit code = %d, want 2", got)
	}
}

func TestSignalGuardStopCleansUpWithoutSignal(t *testing.T) {
	guard := NewSignalGuard().WithExit(func(code int) {
		t.Fatalf("exit invoked unexpectedly with code %d", code)
	})
	ctx, stop := guard.Install(context.Background())

	stop()

	select {
	case <-ctx.Done():
		// stop cancels the derived context as part of cleanup
	case <-time.After(1 * time.Second):
		t.Fatal("ctx not cancelled after stop()")
	}

	// Calling stop twice is idempotent.
	stop()
}

func TestSignalGuardSIGTERMCancelsContext(t *testing.T) {
	guard := NewSignalGuard()
	ctx, stop := guard.Install(context.Background())
	defer stop()

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("kill self with SIGTERM: %v", err)
	}

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("ctx not cancelled within 2s of SIGTERM")
	}
}

func TestSignalGuardParentCancelPropagates(t *testing.T) {
	parent, parentCancel := context.WithCancel(context.Background())
	guard := NewSignalGuard()
	ctx, stop := guard.Install(parent)
	defer stop()

	parentCancel()

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("child ctx not cancelled when parent cancelled")
	}
}
