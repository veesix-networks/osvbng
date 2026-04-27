// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package netbind

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func bindControl(b Binding) func(network, address string, c syscall.RawConn) error {
	if b.VRF == "" {
		return nil
	}
	vrf := b.VRF
	return func(network, address string, c syscall.RawConn) error {
		var setErr error
		if err := c.Control(func(fd uintptr) {
			setErr = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, vrf)
		}); err != nil {
			return fmt.Errorf("netbind: socket control: %w", err)
		}
		if setErr != nil {
			return fmt.Errorf("netbind: SO_BINDTODEVICE %q: %w", vrf, setErr)
		}
		return nil
	}
}
