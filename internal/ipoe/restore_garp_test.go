// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ipoe

import (
	"net"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ha"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

type garpRecorder struct {
	fakeSRGProvider
	requests chan string
}

func (g *garpRecorder) RequestGARP(srg string) { g.requests <- srg }

func promotion(srg string) events.Event {
	return events.Event{Data: events.HAStateChangeEvent{
		SRGName:  srg,
		OldState: string(ha.SRGStateStandby),
		NewState: string(ha.SRGStateActive),
	}}
}

func heldSession(srg string) *SessionState {
	return &SessionState{
		SessionID: "held",
		MAC:       net.HardwareAddr{2, 0, 0, 0, 0, 1},
		State:     "bound",
		SRGName:   srg,
		IPv4:      net.IPv4(100, 64, 0, 2),
	}
}

// A node promoted back after a switchover holds its sessions live and
// has no synced checkpoints to restore (no opdb here, so the restore
// pass returns at once). It must still ask for the GARP flood.
func TestPromotionRequestsGARPWithNothingToRestore(t *testing.T) {
	rec := &garpRecorder{requests: make(chan string, 1)}
	c := &Component{logger: logger.NewTest(), srgMgr: rec}
	c.sessions.Store("held", heldSession("default"))

	c.handleHAStateChange(promotion("default"))

	select {
	case srg := <-rec.requests:
		if srg != "default" {
			t.Fatalf("GARP requested for %q, want default", srg)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("promotion with nothing to restore requested no GARP flood")
	}
}

// Every access component sees the same promotion and each request
// restarts the flood, so a component with no sessions in the SRG, or
// one seeing a transition that is not a promotion, stays quiet.
func TestGARPNotRequestedWithoutSessionsOrPromotion(t *testing.T) {
	rec := &garpRecorder{requests: make(chan string, 4)}
	c := &Component{logger: logger.NewTest(), srgMgr: rec}
	c.sessions.Store("other", heldSession("other-srg"))

	c.handleHAStateChange(promotion("default"))
	for _, tr := range [][2]string{
		{string(ha.SRGStateActive), string(ha.SRGStateStandby)},
		{string(ha.SRGStateActive), string(ha.SRGStateActiveSolo)},
	} {
		c.handleHAStateChange(events.Event{Data: events.HAStateChangeEvent{
			SRGName: "other-srg", OldState: tr[0], NewState: tr[1],
		}})
	}

	select {
	case srg := <-rec.requests:
		t.Fatalf("GARP requested for %q with no sessions there or no promotion", srg)
	case <-time.After(300 * time.Millisecond):
	}
}
