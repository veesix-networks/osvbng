// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ha

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	hapb "github.com/veesix-networks/osvbng/api/proto/ha"
	"github.com/veesix-networks/osvbng/pkg/allocator"
	"github.com/veesix-networks/osvbng/pkg/opdb"
	"google.golang.org/protobuf/proto"
)

type SyncReceiver struct {
	opdb     opdb.Store
	registry *allocator.Registry
	logger   *slog.Logger

	lastSeq    map[string]uint64
	lastRecvNs map[string]int64
	mu         sync.Mutex
}

func NewSyncReceiver(store opdb.Store, registry *allocator.Registry, logger *slog.Logger) *SyncReceiver {
	return &SyncReceiver{
		opdb:       store,
		registry:   registry,
		logger:     logger,
		lastSeq:    make(map[string]uint64),
		lastRecvNs: make(map[string]int64),
	}
}

func (r *SyncReceiver) HandleSyncSession(ctx context.Context, req *hapb.SyncSessionRequest) (*hapb.SyncSessionResponse, error) {
	r.mu.Lock()
	expected := r.lastSeq[req.SrgName] + 1
	if req.Sequence > expected && expected > 1 {
		r.logger.Warn("Sync sequence gap detected",
			"srg", req.SrgName,
			"expected", expected,
			"got", req.Sequence)
	}
	r.lastSeq[req.SrgName] = req.Sequence
	r.lastRecvNs[req.SrgName] = time.Now().UnixNano()
	lastSeq := req.Sequence
	r.mu.Unlock()

	r.logger.Debug("Sync session received",
		"srg", req.SrgName,
		"seq", req.Sequence,
		"action", req.Action.String(),
		"session", req.Session.SessionId,
		"type", req.Session.AccessType)

	switch req.Action {
	case hapb.SyncAction_SYNC_ACTION_CREATE, hapb.SyncAction_SYNC_ACTION_UPDATE:
		if err := r.storeCheckpoint(ctx, req.Session); err != nil {
			return &hapb.SyncSessionResponse{Success: false, LastSyncSeq: lastSeq}, err
		}
		r.reserveAddresses(req.Session)
	case hapb.SyncAction_SYNC_ACTION_DELETE:
		if err := r.deleteCheckpoint(ctx, req.Session); err != nil {
			return &hapb.SyncSessionResponse{Success: false, LastSyncSeq: lastSeq}, err
		}
		r.releaseAddresses(req.Session)
	}

	return &hapb.SyncSessionResponse{Success: true, LastSyncSeq: lastSeq}, nil
}

func (r *SyncReceiver) HandleBulkSyncPage(ctx context.Context, resp *hapb.BulkSyncResponse) error {
	r.logger.Debug("Bulk sync page received",
		"srg", resp.SrgName,
		"sessions", len(resp.Sessions),
		"seq", resp.Sequence,
		"last_page", resp.LastPage)

	for _, cp := range resp.Sessions {
		if err := r.storeCheckpoint(ctx, cp); err != nil {
			return err
		}
		r.reserveAddresses(cp)
	}

	if resp.Sequence > 0 {
		r.mu.Lock()
		r.lastSeq[resp.SrgName] = resp.Sequence
		r.mu.Unlock()
	}

	return nil
}

func (r *SyncReceiver) ClearSyncedNamespace(ctx context.Context, srgName string) error {
	if err := r.opdb.Clear(ctx, opdb.NamespaceHASyncedIPoE); err != nil {
		return fmt.Errorf("clear synced ipoe: %w", err)
	}
	if err := r.opdb.Clear(ctx, opdb.NamespaceHASyncedPPPoE); err != nil {
		return fmt.Errorf("clear synced pppoe: %w", err)
	}
	return nil
}

func (r *SyncReceiver) GetLastSeq(srgName string) uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastSeq[srgName]
}

func (r *SyncReceiver) GetLastRecvTime(srgName string) time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	ns := r.lastRecvNs[srgName]
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

func (r *SyncReceiver) storeCheckpoint(ctx context.Context, cp *hapb.SessionCheckpoint) error {
	data, err := proto.Marshal(cp)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	ns := opdb.NamespaceHASyncedIPoE
	if cp.AccessType == "pppoe" {
		ns = opdb.NamespaceHASyncedPPPoE
	}

	return r.opdb.Put(ctx, ns, cp.SessionId, data)
}

func (r *SyncReceiver) deleteCheckpoint(ctx context.Context, cp *hapb.SessionCheckpoint) error {
	ns := opdb.NamespaceHASyncedIPoE
	if cp.AccessType == "pppoe" {
		ns = opdb.NamespaceHASyncedPPPoE
	}

	return r.opdb.Delete(ctx, ns, cp.SessionId)
}

func (r *SyncReceiver) reserveAddresses(cp *hapb.SessionCheckpoint) {
	if r.registry == nil {
		return
	}
	if len(cp.Ipv4Address) > 0 {
		var err error
		if cp.Ipv4Pool != "" {
			err = r.registry.ReserveIPInPool(cp.Ipv4Pool, net.IP(cp.Ipv4Address), cp.SessionId)
		} else {
			err = r.registry.ReserveIP(net.IP(cp.Ipv4Address), cp.SessionId)
		}
		if err != nil {
			r.logger.Debug("Failed to reserve IPv4 from sync", "session", cp.SessionId, "error", err)
		}
	}
	if len(cp.Ipv6Address) > 0 {
		var err error
		if cp.IanaPool != "" {
			err = r.registry.ReserveIANAInPool(cp.IanaPool, net.IP(cp.Ipv6Address), cp.SessionId)
		} else {
			err = r.registry.ReserveIANA(net.IP(cp.Ipv6Address), cp.SessionId)
		}
		if err != nil {
			r.logger.Debug("Failed to reserve IANA from sync", "session", cp.SessionId, "error", err)
		}
	}
	if len(cp.Ipv6Prefix) > 0 && cp.Ipv6PrefixLen > 0 {
		ipNet := &net.IPNet{
			IP:   net.IP(cp.Ipv6Prefix),
			Mask: net.CIDRMask(int(cp.Ipv6PrefixLen), 128),
		}
		var err error
		if cp.PdPool != "" {
			err = r.registry.ReservePDInPool(cp.PdPool, ipNet, cp.SessionId)
		} else {
			err = r.registry.ReservePD(ipNet, cp.SessionId)
		}
		if err != nil {
			r.logger.Debug("Failed to reserve PD from sync", "session", cp.SessionId, "error", err)
		}
	}
}

func (r *SyncReceiver) releaseAddresses(cp *hapb.SessionCheckpoint) {
	if r.registry == nil {
		return
	}
	if len(cp.Ipv4Address) > 0 {
		r.registry.ReleaseIP(net.IP(cp.Ipv4Address))
	}
	if len(cp.Ipv6Address) > 0 {
		r.registry.ReleaseIANAByIP(net.IP(cp.Ipv6Address))
	}
	if len(cp.Ipv6Prefix) > 0 && cp.Ipv6PrefixLen > 0 {
		ipNet := &net.IPNet{
			IP:   net.IP(cp.Ipv6Prefix),
			Mask: net.CIDRMask(int(cp.Ipv6PrefixLen), 128),
		}
		r.registry.ReleasePDByPrefix(ipNet)
	}
}
