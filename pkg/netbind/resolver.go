// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package netbind

import (
	"context"
	"net"
)

func Resolver(b Binding) *net.Resolver {
	if b.IsZero() {
		return net.DefaultResolver
	}

	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := &net.Dialer{Control: bindControl(b)}
			if b.SourceIP.IsValid() {
				d.LocalAddr = sourceLocalAddr(network, b.SourceIP)
			}
			return d.DialContext(ctx, network, address)
		},
	}
}
