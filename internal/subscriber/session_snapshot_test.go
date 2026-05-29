// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package subscriber

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/veesix-networks/osvbng/pkg/models"
)

type memCache struct {
	m map[string][]byte
}

func newMemCache() *memCache { return &memCache{m: map[string][]byte{}} }

func (c *memCache) Set(ctx context.Context, k string, v []byte, ttl time.Duration) error {
	c.m[k] = append([]byte(nil), v...)
	return nil
}
func (c *memCache) Get(ctx context.Context, k string) ([]byte, error) {
	v, ok := c.m[k]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", k)
	}
	return v, nil
}
func (c *memCache) GetAll(ctx context.Context, pattern string) (map[string][]byte, error) {
	return nil, nil
}
func (c *memCache) Delete(ctx context.Context, k string) error { delete(c.m, k); return nil }
func (c *memCache) Scan(ctx context.Context, cursor uint64, pattern string, count int64) ([]string, uint64, error) {
	return nil, 0, nil
}
func (c *memCache) Incr(ctx context.Context, k string) (int64, error)             { return 0, nil }
func (c *memCache) Decr(ctx context.Context, k string) (int64, error)             { return 0, nil }
func (c *memCache) Expire(ctx context.Context, k string, ttl time.Duration) error { return nil }
func (c *memCache) Close() error                                                   { return nil }

func TestDecodeTypedSession_PPPoEHasClassificationMetadata(t *testing.T) {
	src := &models.PPPSession{
		SessionID:    "s-ppp-1",
		State:        models.SessionStateActive,
		AccessType:   string(models.AccessTypePPPoE),
		Protocol:     string(models.ProtocolPPPoESession),
		IfIndex:      77,
		VRF:          "vrf-blue",
		ServiceGroup: "residential",
		SRGName:      "default",
	}
	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	sess, ok := decodeTypedSession(string(models.AccessTypePPPoE), data)
	if !ok {
		t.Fatalf("decodeTypedSession returned false")
	}
	pp, ok := sess.(*models.PPPSession)
	if !ok {
		t.Fatalf("expected *models.PPPSession, got %T", sess)
	}
	if pp.IfIndex != 77 {
		t.Fatalf("IfIndex = %d, want 77", pp.IfIndex)
	}
	if pp.VRF != "vrf-blue" || pp.ServiceGroup != "residential" || pp.SRGName != "default" {
		t.Fatalf("classification fields lost: VRF=%q SG=%q SRG=%q", pp.VRF, pp.ServiceGroup, pp.SRGName)
	}
	if pp.AccessType != string(models.AccessTypePPPoE) || pp.Protocol != string(models.ProtocolPPPoESession) {
		t.Fatalf("AccessType/Protocol lost: %q/%q", pp.AccessType, pp.Protocol)
	}
}

func TestDecodeTypedSession_IPoEDefault(t *testing.T) {
	src := &models.IPoESession{
		SessionID:  "s-ipoe-1",
		State:      models.SessionStateActive,
		AccessType: string(models.AccessTypeIPoE),
		IfIndex:    42,
		VRF:        "default",
	}
	data, _ := json.Marshal(src)
	sess, ok := decodeTypedSession(string(models.AccessTypeIPoE), data)
	if !ok {
		t.Fatalf("decodeTypedSession returned false")
	}
	if _, ok := sess.(*models.IPoESession); !ok {
		t.Fatalf("expected *models.IPoESession, got %T", sess)
	}
}

func TestSessionSnapshot_PicksPPPoEAndPreservesIfIndex(t *testing.T) {
	mc := newMemCache()
	src := &models.PPPSession{
		SessionID:  "s1",
		AccessType: string(models.AccessTypePPPoE),
		IfIndex:    99,
	}
	data, _ := json.Marshal(src)
	mc.Set(context.Background(), "osvbng:sessions:s1", data, 0)

	c := &Component{cache: mc}
	sess, ok := c.SessionSnapshot(context.Background(), "s1")
	if !ok {
		t.Fatalf("expected snapshot found")
	}
	pp, ok := sess.(*models.PPPSession)
	if !ok {
		t.Fatalf("expected *models.PPPSession, got %T", sess)
	}
	if pp.IfIndex != 99 {
		t.Fatalf("IfIndex = %d, want 99", pp.IfIndex)
	}
}

func TestSessionSnapshot_AbsentReturnsFalse(t *testing.T) {
	c := &Component{cache: newMemCache()}
	_, ok := c.SessionSnapshot(context.Background(), "missing")
	if ok {
		t.Fatalf("expected false for missing session")
	}
}
