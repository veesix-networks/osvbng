// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import "fmt"

type Config struct {
	Standalone bool             `json:"standalone,omitempty" yaml:"standalone,omitempty"`
	Pools      map[string]*Pool `json:"pools,omitempty" yaml:"pools,omitempty"`
	Logging    *LoggingConfig   `json:"logging,omitempty" yaml:"logging,omitempty"`
}

type Pool struct {
	Mode                     string         `json:"mode,omitempty" yaml:"mode,omitempty"`
	AutoConfigure            bool           `json:"auto-configure,omitempty" yaml:"auto-configure,omitempty"`
	InsidePrefixes           []InsidePrefix `json:"inside-prefixes,omitempty" yaml:"inside-prefixes,omitempty"`
	OutsideAddresses         []string       `json:"outside-addresses,omitempty" yaml:"outside-addresses,omitempty"`
	BlockSize                uint16         `json:"block-size,omitempty" yaml:"block-size,omitempty"`
	MaxBlocksPerSubscriber   uint8          `json:"max-blocks-per-subscriber,omitempty" yaml:"max-blocks-per-subscriber,omitempty"`
	MaxSessionsPerSubscriber uint32         `json:"max-sessions-per-subscriber,omitempty" yaml:"max-sessions-per-subscriber,omitempty"`
	ExhaustionBehavior       string         `json:"exhaustion-behavior,omitempty" yaml:"exhaustion-behavior,omitempty"`
	PortReuseTimeout         uint16         `json:"port-reuse-timeout,omitempty" yaml:"port-reuse-timeout,omitempty"`
	SubscriberRatio          uint16         `json:"subscriber-ratio,omitempty" yaml:"subscriber-ratio,omitempty"`
	PortsPerSubscriber       uint16         `json:"ports-per-subscriber,omitempty" yaml:"ports-per-subscriber,omitempty"`
	PortRange                string         `json:"port-range,omitempty" yaml:"port-range,omitempty"`
	AddressPooling           string         `json:"address-pooling,omitempty" yaml:"address-pooling,omitempty"`
	Filtering                string         `json:"filtering,omitempty" yaml:"filtering,omitempty"`
	ExcludedAddresses        []string       `json:"excluded-addresses,omitempty" yaml:"excluded-addresses,omitempty"`
	BlacklistMode            string         `json:"blacklist-mode,omitempty" yaml:"blacklist-mode,omitempty"`
	NetworkRoutePolicy       string         `json:"network-route-policy,omitempty" yaml:"network-route-policy,omitempty"`
	ALG                      *ALGConfig     `json:"alg,omitempty" yaml:"alg,omitempty"`
	Timeouts                 *TimeoutConfig `json:"timeouts,omitempty" yaml:"timeouts,omitempty"`
}

type InsidePrefix struct {
	Prefix string `json:"prefix" yaml:"prefix"`
	VRF    string `json:"vrf,omitempty" yaml:"vrf,omitempty"`
}

type ALGConfig struct {
	FTP  *bool `json:"ftp,omitempty" yaml:"ftp,omitempty"`
	TFTP *bool `json:"tftp,omitempty" yaml:"tftp,omitempty"`
	PPTP *bool `json:"pptp,omitempty" yaml:"pptp,omitempty"`
	SIP  *bool `json:"sip,omitempty" yaml:"sip,omitempty"`
	RTSP *bool `json:"rtsp,omitempty" yaml:"rtsp,omitempty"`
	DNS  *bool `json:"dns,omitempty" yaml:"dns,omitempty"`
}

type TimeoutConfig struct {
	TCPEstablished uint32 `json:"tcp-established,omitempty" yaml:"tcp-established,omitempty"`
	TCPTransitory  uint32 `json:"tcp-transitory,omitempty" yaml:"tcp-transitory,omitempty"`
	UDP            uint32 `json:"udp,omitempty" yaml:"udp,omitempty"`
	ICMP           uint32 `json:"icmp,omitempty" yaml:"icmp,omitempty"`
}

type LoggingConfig struct {
	Enabled          bool              `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Format           string            `json:"format,omitempty" yaml:"format,omitempty"`
	Collectors       []CollectorConfig `json:"collectors,omitempty" yaml:"collectors,omitempty"`
	TemplateInterval uint32            `json:"template-interval,omitempty" yaml:"template-interval,omitempty"`
	Syslog           *SyslogConfig     `json:"syslog,omitempty" yaml:"syslog,omitempty"`
}

type CollectorConfig struct {
	Address  string `json:"address" yaml:"address"`
	Source   string `json:"source,omitempty" yaml:"source,omitempty"`
	DomainID uint32 `json:"domain-id,omitempty" yaml:"domain-id,omitempty"`
}

type SyslogConfig struct {
	Server   string `json:"server" yaml:"server"`
	Facility string `json:"facility,omitempty" yaml:"facility,omitempty"`
}

func (p *Pool) GetMode() string {
	if p.Mode == "" {
		return "pba"
	}
	return p.Mode
}

func (p *Pool) GetBlockSize() uint16 {
	if p.BlockSize > 0 {
		return p.BlockSize
	}
	if p.SubscriberRatio > 0 {
		usable := p.GetPortRangeSize()
		return uint16(usable / uint32(p.SubscriberRatio))
	}
	return 512
}

func (p *Pool) GetPortRangeStart() uint16 {
	if p.PortRange != "" {
		start, _ := parsePortRange(p.PortRange)
		return start
	}
	return 1024
}

func (p *Pool) GetPortRangeEnd() uint16 {
	if p.PortRange != "" {
		_, end := parsePortRange(p.PortRange)
		return end
	}
	return 65535
}

func (p *Pool) GetPortRangeSize() uint32 {
	return uint32(p.GetPortRangeEnd()) - uint32(p.GetPortRangeStart()) + 1
}

func (p *Pool) GetMaxBlocksPerSubscriber() uint8 {
	if p.MaxBlocksPerSubscriber > 0 {
		return p.MaxBlocksPerSubscriber
	}
	return 4
}

func (p *Pool) GetMaxSessionsPerSubscriber() uint32 {
	if p.MaxSessionsPerSubscriber > 0 {
		return p.MaxSessionsPerSubscriber
	}
	return 2000
}

func (p *Pool) GetPortReuseTimeout() uint16 {
	if p.PortReuseTimeout > 0 {
		return p.PortReuseTimeout
	}
	return 120
}

func (p *Pool) GetAddressPooling() string {
	if p.AddressPooling == "" {
		return "paired"
	}
	return p.AddressPooling
}

func (p *Pool) GetFiltering() string {
	if p.Filtering == "" {
		return "endpoint-independent"
	}
	return p.Filtering
}

func (p *Pool) GetBlacklistMode() string {
	if p.BlacklistMode == "" {
		return "new-only"
	}
	return p.BlacklistMode
}

func (p *Pool) GetExhaustionBehavior() string {
	if p.ExhaustionBehavior == "" {
		return "drop-icmp"
	}
	return p.ExhaustionBehavior
}

func (p *Pool) GetTimeouts() TimeoutConfig {
	t := TimeoutConfig{
		TCPEstablished: 7200,
		TCPTransitory:  240,
		UDP:            300,
		ICMP:           60,
	}
	if p.Timeouts != nil {
		if p.Timeouts.TCPEstablished > 0 {
			t.TCPEstablished = p.Timeouts.TCPEstablished
		}
		if p.Timeouts.TCPTransitory > 0 {
			t.TCPTransitory = p.Timeouts.TCPTransitory
		}
		if p.Timeouts.UDP > 0 {
			t.UDP = p.Timeouts.UDP
		}
		if p.Timeouts.ICMP > 0 {
			t.ICMP = p.Timeouts.ICMP
		}
	}
	return t
}

func (p *Pool) GetALGBitmask() uint8 {
	if p.ALG == nil {
		return 0x1F
	}
	var mask uint8
	if p.ALG.FTP == nil || *p.ALG.FTP {
		mask |= 0x01
	}
	if p.ALG.TFTP == nil || *p.ALG.TFTP {
		mask |= 0x02
	}
	if p.ALG.PPTP == nil || *p.ALG.PPTP {
		mask |= 0x04
	}
	if p.ALG.SIP == nil || *p.ALG.SIP {
		mask |= 0x08
	}
	if p.ALG.RTSP == nil || *p.ALG.RTSP {
		mask |= 0x10
	}
	if p.ALG.DNS != nil && *p.ALG.DNS {
		mask |= 0x20
	}
	return mask
}

func parsePortRange(s string) (uint16, uint16) {
	var start, end uint16
	n, _ := fmt.Sscanf(s, "%d-%d", &start, &end)
	if n == 2 {
		return start, end
	}
	return 1024, 65535
}
