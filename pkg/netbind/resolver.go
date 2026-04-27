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

	dnsDialer := &net.Dialer{
		Control: bindControl(b),
	}
	if b.SourceIP.IsValid() {
		dnsDialer.LocalAddr = sourceTCPAddr(b.SourceIP)
	}

	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dnsDialer.DialContext(ctx, network, address)
		},
	}
}
