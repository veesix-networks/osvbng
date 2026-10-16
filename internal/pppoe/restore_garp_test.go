// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package pppoe

import (
	"net"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/events"
	"github.com/veesix-networks/osvbng/pkg/ha"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/ppp"
)

type garpRecorder struct {
	requests chan string
}

func (g *garpRecorder) GetVirtualMAC(string) net.HardwareAddr { return nil }
func (g *garpRecorder) IsActive(string) bool                  { return true }
func (g *garpRecorder) GetSRGForGroup(string) string          { return "" }
func (g *garpRecorder) RequestGARP(srg string)                { g.requests <- srg }

func promotion(srg string) events.Event {
	return events.Event{Data: events.HAStateChangeEvent{
		SRGName:  srg,
		OldState: string(ha.SRGStateStandby),
		NewState: string(ha.SRGStateActive),
	}}
}

func componentHolding(srg string, rec *garpRecorder) *Component {
	c := &Component{
		logger:   logger.NewTest(),
		srgMgr:   rec,
		sessions: map[string]*SessionState{},
	}
	c.sessions["held"] = &SessionState{
		SessionID: "held",
		MAC:       net.HardwareAddr{2, 0, 0, 0, 0, 1},
		Phase:     ppp.PhaseOpen,
		SRGName:   srg,
	}
	return c
}

// A PPPoE-only SRG never asked for the GARP flood on promotion. With no
// opdb the restore pass returns at once; the request must still follow
// when this component holds sessions in the SRG.
func TestPromotionRequestsGARPWithNothingToRestore(t *testing.T) {
	rec := &garpRecorder{requests: make(chan string, 1)}
	c := componentHolding("default", rec)

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

// With no PPPoE sessions in the promoted SRG the request is left to the
// component that holds them, so the flood is not restarted for nothing.
func TestGARPNotRequestedWithoutSessionsInSRG(t *testing.T) {
	rec := &garpRecorder{requests: make(chan string, 1)}
	c := componentHolding("other-srg", rec)

	c.handleHAStateChange(promotion("default"))

	select {
	case srg := <-rec.requests:
		t.Fatalf("GARP requested for %q with no sessions there", srg)
	case <-time.After(300 * time.Millisecond):
	}
}
