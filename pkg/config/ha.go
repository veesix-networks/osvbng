// Copyright 2025 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/veesix-networks/osvbng/pkg/netbind"
)

type HAConfig struct {
	Enabled   bool                  `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	NodeID    string                `json:"node_id,omitempty" yaml:"node_id,omitempty"`
	Listen    HAListenConfig        `json:"listen,omitempty" yaml:"listen,omitempty"`
	Peer      HAPeerConfig          `json:"peer,omitempty" yaml:"peer,omitempty"`
	TLS       HATLSConfig           `json:"tls,omitempty" yaml:"tls,omitempty"`
	Heartbeat HAHeartbeatConfig     `json:"heartbeat,omitempty" yaml:"heartbeat,omitempty"`
	Sync      HASyncConfig          `json:"sync,omitempty" yaml:"sync,omitempty"`
	SRGs      map[string]*SRGConfig `json:"srgs,omitempty" yaml:"srgs,omitempty"`
}

type HASyncConfig struct {
	BacklogSize     int           `json:"backlog_size,omitempty" yaml:"backlog_size,omitempty"`
	PageSize        int           `json:"page_size,omitempty" yaml:"page_size,omitempty"`
	BulkSyncTimeout time.Duration `json:"bulk_sync_timeout,omitempty" yaml:"bulk_sync_timeout,omitempty"`
}

type HATLSConfig struct {
	CACert string `json:"ca_cert,omitempty" yaml:"ca_cert,omitempty"`
	Cert   string `json:"cert,omitempty" yaml:"cert,omitempty"`
	Key    string `json:"key,omitempty" yaml:"key,omitempty"`
}

type HAListenConfig struct {
	netbind.EndpointBinding `json:",inline" yaml:",inline"`

	Address string `json:"address,omitempty" yaml:"address,omitempty"`
}

type HAPeerConfig struct {
	netbind.EndpointBinding `json:",inline" yaml:",inline"`

	Address string `json:"address,omitempty" yaml:"address,omitempty"`
}

type HAHeartbeatConfig struct {
	Interval       time.Duration `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout        time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	StartupTimeout time.Duration `json:"startup_timeout,omitempty" yaml:"startup_timeout,omitempty"`
}

type SRGNetwork struct {
	Prefix string `json:"prefix" yaml:"prefix"`
	VRF    string `json:"vrf,omitempty" yaml:"vrf,omitempty"`
}

type SRGGARPConfig struct {
	Enabled        *bool         `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	BatchSize      int           `json:"batch-size,omitempty" yaml:"batch-size,omitempty"`
	BatchDelay     time.Duration `json:"batch-delay,omitempty" yaml:"batch-delay,omitempty"`
	RepeatCount    int           `json:"repeat-count,omitempty" yaml:"repeat-count,omitempty"`
	RepeatInterval time.Duration `json:"repeat-interval,omitempty" yaml:"repeat-interval,omitempty"`
}

func (g *SRGGARPConfig) IsEnabled() bool {
	if g == nil || g.Enabled == nil {
		return true
	}
	return *g.Enabled
}

func (g *SRGGARPConfig) GetBatchSize() int {
	if g == nil || g.BatchSize <= 0 {
		return 256
	}
	return g.BatchSize
}

func (g *SRGGARPConfig) GetBatchDelay() time.Duration {
	if g == nil || g.BatchDelay <= 0 {
		return time.Millisecond
	}
	return g.BatchDelay
}

func (g *SRGGARPConfig) GetRepeatCount() int {
	if g == nil || g.RepeatCount <= 0 {
		return 1
	}
	return g.RepeatCount
}

func (g *SRGGARPConfig) GetRepeatInterval() time.Duration {
	if g == nil || g.RepeatInterval <= 0 {
		return 100 * time.Millisecond
	}
	return g.RepeatInterval
}

type SRGConfig struct {
	VirtualMAC             string        `json:"virtual_mac,omitempty" yaml:"virtual_mac,omitempty"`
	Priority               uint32        `json:"priority,omitempty" yaml:"priority,omitempty"`
	Preempt                bool          `json:"preempt,omitempty" yaml:"preempt,omitempty"`
	SubscriberGroups       []string      `json:"subscriber_groups,omitempty" yaml:"subscriber_groups,omitempty"`
	Interfaces             []string      `json:"interfaces,omitempty" yaml:"interfaces,omitempty"`
	TrackPriorityDecrement uint32        `json:"track_priority_decrement,omitempty" yaml:"track_priority_decrement,omitempty"`
	Networks               []SRGNetwork  `json:"networks,omitempty" yaml:"networks,omitempty"`
	GARP                   *SRGGARPConfig `json:"garp,omitempty" yaml:"garp,omitempty"`
}

func (c *HAConfig) GetListenAddress() string {
	if c.Listen.Address != "" {
		return c.Listen.Address
	}
	return ":50051"
}

func (c *HAConfig) GetHeartbeatInterval() time.Duration {
	if c.Heartbeat.Interval > 0 {
		return c.Heartbeat.Interval
	}
	return time.Second
}

func (c *HAConfig) GetHeartbeatTimeout() time.Duration {
	if c.Heartbeat.Timeout > 0 {
		return c.Heartbeat.Timeout
	}
	return 5 * time.Second
}

func (c *HAConfig) GetSyncBacklogSize() int {
	if c.Sync.BacklogSize > 0 {
		return c.Sync.BacklogSize
	}
	return 10000
}

func (c *HAConfig) GetSyncPageSize() int {
	if c.Sync.PageSize > 0 {
		return c.Sync.PageSize
	}
	return 1000
}

func (c *HAConfig) GetBulkSyncTimeout() time.Duration {
	if c.Sync.BulkSyncTimeout > 0 {
		return c.Sync.BulkSyncTimeout
	}
	return 60 * time.Second
}

func (c *HAConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.NodeID == "" {
		return fmt.Errorf("ha.node_id is required")
	}

	if c.Peer.Address == "" {
		return fmt.Errorf("ha.peer.address is required")
	}

	if _, _, err := net.SplitHostPort(c.Peer.Address); err != nil {
		return fmt.Errorf("ha.peer.address: %w", err)
	}

	if c.Listen.Address != "" {
		if _, _, err := net.SplitHostPort(c.Listen.Address); err != nil {
			return fmt.Errorf("ha.listen.address: %w", err)
		}
	}

	if len(c.SRGs) == 0 {
		return fmt.Errorf("ha.srgs: at least one SRG must be configured")
	}

	for name, srg := range c.SRGs {
		if srg.Priority < 1 || srg.Priority > 255 {
			return fmt.Errorf("ha.srgs.%s.priority: must be 1-255, got %d", name, srg.Priority)
		}
		if srg.VirtualMAC != "" {
			if _, err := net.ParseMAC(srg.VirtualMAC); err != nil {
				return fmt.Errorf("ha.srgs.%s.virtual_mac: %w", name, err)
			}
		}
		if len(srg.SubscriberGroups) == 0 {
			return fmt.Errorf("ha.srgs.%s.subscriber_groups: at least one subscriber group is required", name)
		}
	}

	if c.TLS.CACert != "" || c.TLS.Cert != "" || c.TLS.Key != "" {
		if c.TLS.CACert == "" || c.TLS.Cert == "" || c.TLS.Key == "" {
			return fmt.Errorf("ha.tls: all three fields (ca_cert, cert, key) must be set together")
		}
		for _, f := range []struct{ name, path string }{
			{"ca_cert", c.TLS.CACert},
			{"cert", c.TLS.Cert},
			{"key", c.TLS.Key},
		} {
			if _, err := os.Stat(f.path); err != nil {
				return fmt.Errorf("ha.tls.%s: %w", f.name, err)
			}
		}
	}

	if c.Heartbeat.Timeout > 0 && c.Heartbeat.Interval > 0 {
		if c.Heartbeat.Timeout <= c.Heartbeat.Interval {
			return fmt.Errorf("ha.heartbeat.timeout (%s) must be greater than interval (%s)", c.Heartbeat.Timeout, c.Heartbeat.Interval)
		}
	}

	return nil
}
