// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package netbind

import (
	"fmt"
	"runtime"
	"sync/atomic"

	"github.com/vishvananda/netns"
)

var lcpNetNs atomic.Value

// SetLCPNetNs records the netns name (e.g. "dataplane") where
// vrfmgr creates VRF master devices. VRF-bound sockets are opened
// inside that netns and retain it for life; the calling goroutine
// returns to its original netns once the socket is open. The empty
// string disables the netns dance — sockets open in the calling
// thread's current netns. Set once at daemon startup.
func SetLCPNetNs(name string) {
	lcpNetNs.Store(name)
}

func currentLCPNetNs() string {
	v := lcpNetNs.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}

// withNetNS runs fn with the calling OS thread temporarily switched
// into the configured LCP netns. If no LCP netns is configured, or
// the binding has no VRF, fn runs directly with no thread lock and
// no netns syscall — preserving the zero-binding fast path.
//
// The thread is restored to its original netns before this returns.
// If restoration fails the thread is left locked so the Go runtime
// will not reuse it for unrelated goroutines.
func withNetNS(b Binding, fn func() error) error {
	if b.VRF == "" {
		return fn()
	}
	nsName := currentLCPNetNs()
	if nsName == "" {
		return fn()
	}

	runtime.LockOSThread()

	origNs, err := netns.Get()
	if err != nil {
		runtime.UnlockOSThread()
		return fmt.Errorf("netbind: get current netns: %w", err)
	}
	defer origNs.Close()

	targetNs, err := netns.GetFromName(nsName)
	if err != nil {
		runtime.UnlockOSThread()
		return fmt.Errorf("netbind: get netns %q: %w", nsName, err)
	}
	defer targetNs.Close()

	if err := netns.Set(targetNs); err != nil {
		runtime.UnlockOSThread()
		return fmt.Errorf("netbind: enter netns %q: %w", nsName, err)
	}

	fnErr := fn()

	if setErr := netns.Set(origNs); setErr != nil {
		// Thread is stuck in the wrong netns. Do NOT unlock — the
		// runtime would hand this corrupted thread to an unrelated
		// goroutine. Leak the thread and return both errors.
		if fnErr != nil {
			return fmt.Errorf("netbind: %w (and restore netns failed: %v)", fnErr, setErr)
		}
		return fmt.Errorf("netbind: restore netns: %w", setErr)
	}
	runtime.UnlockOSThread()
	return fnErr
}
