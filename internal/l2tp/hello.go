// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package l2tp

import (
	"time"

	l2tppkt "github.com/veesix-networks/osvbng/pkg/l2tp"
)

// HelloScheduler fires Hello control messages onto the supplied
// control channel at `interval` cadence. The owning tunnel runner
// drives Stop on teardown. One goroutine per tunnel is acceptable —
// tunnels are sparse (target ~1k per BNG) versus sessions (~64k).
type HelloScheduler struct {
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
}

func NewHelloScheduler(interval time.Duration) *HelloScheduler {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &HelloScheduler{
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

func (h *HelloScheduler) Start(send func([]byte) error) {
	go func() {
		defer close(h.done)
		t := time.NewTicker(h.interval)
		defer t.Stop()
		body := l2tppkt.BuildHello()
		for {
			select {
			case <-h.stop:
				return
			case <-t.C:
				_ = send(body)
			}
		}
	}()
}

func (h *HelloScheduler) Stop() {
	select {
	case <-h.stop:
		return
	default:
		close(h.stop)
	}
	<-h.done
}
