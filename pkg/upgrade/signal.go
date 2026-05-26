// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// SignalGuard converts SIGINT / SIGTERM into ctx-cancellation for the
// duration of an upgrade-in-progress critical section. Without it, the
// default osvbngcli SIGINT handler at main.go would call os.Exit(0),
// bypassing every defer in the upgrade call stack — including the
// "remove transient drop-in" cleanup that restores systemd's normal
// Restart=on-failure policy.
//
// Usage:
//
//	guard := upgrade.NewSignalGuard()
//	ctx, stop := guard.Install(parentCtx)
//	defer stop()
//	// run apply with ctx; first SIGINT cancels ctx, second forces exit
//
// First signal: cancel the upgrade context. The Runner observes ctx.Done()
// and routes through its cleanup paths (Restore systemd, attempt rollback,
// write final journal phase).
//
// Second signal: hard-exit with status 2. Operator has chosen to bail
// regardless of cleanup state. Journal phase will be whatever the first
// signal got us to.
type SignalGuard struct {
	exit func(int)
}

// NewSignalGuard returns a SignalGuard wired to os.Exit. Tests inject
// a custom exit func via WithExit.
func NewSignalGuard() *SignalGuard {
	return &SignalGuard{exit: os.Exit}
}

// WithExit returns a copy of the SignalGuard configured to call the
// supplied exit function on a second signal. Used by tests; production
// uses os.Exit via NewSignalGuard.
func (g *SignalGuard) WithExit(exit func(int)) *SignalGuard {
	clone := *g
	clone.exit = exit
	return &clone
}

// Install registers a signal handler for SIGINT and SIGTERM and
// returns a derived context that will be cancelled on the first
// signal, plus a stop func that the caller defers to unregister the
// handler and release the goroutine.
//
// Calling stop() after the context has already been cancelled is
// safe — the goroutine exits either when a signal arrives or when
// stop is called, whichever comes first.
func (g *SignalGuard) Install(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	stopOnce := sync.Once{}
	doneCh := make(chan struct{})
	stop := func() {
		stopOnce.Do(func() {
			signal.Stop(sigCh)
			close(doneCh)
			cancel()
		})
	}

	go func() {
		defer signal.Stop(sigCh)
		select {
		case <-sigCh:
			cancel()
			// Wait for either a second signal (hard exit) or stop().
			select {
			case <-sigCh:
				g.exit(2)
			case <-doneCh:
				return
			}
		case <-doneCh:
			return
		case <-parent.Done():
			cancel()
			return
		}
	}()

	return ctx, stop
}
