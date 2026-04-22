// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package vpp

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/veesix-networks/osvbng/pkg/events"
	vppinterfaces "github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/interface_types"
	"go.fd.io/govpp/api"
)

type InterfaceWatchSet struct {
	mu    sync.RWMutex
	items map[uint32]bool
}

func NewInterfaceWatchSet() *InterfaceWatchSet {
	return &InterfaceWatchSet{items: make(map[uint32]bool)}
}

func (v *VPP) NewInterfaceWatchSet() *InterfaceWatchSet {
	return NewInterfaceWatchSet()
}

func (w *InterfaceWatchSet) Add(swIfIndex uint32) {
	w.mu.Lock()
	w.items[swIfIndex] = true
	w.mu.Unlock()
}

func (w *InterfaceWatchSet) Contains(swIfIndex uint32) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.items[swIfIndex]
}

func (w *InterfaceWatchSet) Len() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.items)
}

func (v *VPP) StartInterfaceWatcher(ctx context.Context, eventBus events.Bus, watchSet *InterfaceWatchSet) error {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return err
	}

	notifChan := make(chan api.Message, 256)
	sub, err := ch.SubscribeNotification(notifChan, &vppinterfaces.SwInterfaceEvent{})
	if err != nil {
		ch.Close()
		return err
	}

	req := &vppinterfaces.WantInterfaceEvents{
		EnableDisable: 1,
		PID:           uint32(os.Getpid()),
	}
	reply := &vppinterfaces.WantInterfaceEventsReply{}
	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		_ = sub.Unsubscribe()
		ch.Close()
		return err
	}

	go func() {
		defer ch.Close()
		defer sub.Unsubscribe()

		for {
			select {
			case <-ctx.Done():
				disableReq := &vppinterfaces.WantInterfaceEvents{
					EnableDisable: 0,
					PID:           uint32(os.Getpid()),
				}
				disableReply := &vppinterfaces.WantInterfaceEventsReply{}
				_ = ch.SendRequest(disableReq).ReceiveReply(disableReply)
				return

			case msg, ok := <-notifChan:
				if !ok {
					return
				}

				ev, ok := msg.(*vppinterfaces.SwInterfaceEvent)
				if !ok {
					continue
				}

				idx := uint32(ev.SwIfIndex)
				if !watchSet.Contains(idx) {
					continue
				}

				adminUp := ev.Flags&interface_types.IF_STATUS_API_FLAG_ADMIN_UP != 0
				linkUp := ev.Flags&interface_types.IF_STATUS_API_FLAG_LINK_UP != 0

				name := ""
				if iface := v.ifMgr.Get(idx); iface != nil {
					iface.AdminUp = adminUp
					iface.LinkUp = linkUp
					name = iface.Name
				}

				eventBus.Publish(events.TopicInterfaceState, events.Event{
					Type:      "interface.state",
					Timestamp: time.Now(),
					Source:    "vpp",
					Data: events.InterfaceStateEvent{
						SwIfIndex: idx,
						Name:      name,
						AdminUp:   adminUp,
						LinkUp:    linkUp,
						Deleted:   ev.Deleted,
					},
				})

				v.logger.Info("Interface state change",
					"sw_if_index", idx,
					"name", name,
					"admin_up", adminUp,
					"link_up", linkUp,
					"deleted", ev.Deleted)
			}
		}
	}()

	v.logger.Info("VPP interface watcher started", "watched_interfaces", watchSet.Len())
	return nil
}
