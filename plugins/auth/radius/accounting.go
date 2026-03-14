// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package radius

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/veesix-networks/osvbng/pkg/aaa"
	"github.com/veesix-networks/osvbng/pkg/auth"
	"layeh.com/radius"
)

const (
	acctStatusStart         = 1
	acctStatusStop          = 2
	acctStatusInterimUpdate = 3
)

func (p *Provider) StartAccounting(_ context.Context, session *auth.Session) error {
	return p.sendAccounting(session, acctStatusStart)
}

func (p *Provider) UpdateAccounting(_ context.Context, session *auth.Session) error {
	return p.sendAccounting(session, acctStatusInterimUpdate)
}

func (p *Provider) StopAccounting(_ context.Context, session *auth.Session) error {
	return p.sendAccounting(session, acctStatusStop)
}

func (p *Provider) sendAccounting(session *auth.Session, statusType uint32) error {
	packet := radius.New(radius.CodeAccountingRequest, nil)

	packet.Add(40, encodeUint32(statusType))

	if session.AcctSessionID != "" {
		packet.Add(44, radius.Attribute(session.AcctSessionID))
	}
	if session.Username != "" {
		packet.Add(1, radius.Attribute(session.Username))
	}
	if session.MAC != "" {
		packet.Add(31, radius.Attribute(session.MAC))
	}

	if p.cfg.NASIdentifier != "" {
		packet.Add(32, radius.Attribute(p.cfg.NASIdentifier))
	}
	if p.cfg.NASIP != "" {
		if ip := net.ParseIP(p.cfg.NASIP); ip != nil {
			packet.Add(4, radius.Attribute(ip.To4()))
		}
	}

	packet.Add(46, encodeUint32(uint32(session.SessionDuration)))
	packet.Add(42, encodeUint32(uint32(session.RxBytes)))
	packet.Add(43, encodeUint32(uint32(session.TxBytes)))
	if rxGiga := uint32(session.RxBytes >> 32); rxGiga > 0 {
		packet.Add(52, encodeUint32(rxGiga))
	}
	if txGiga := uint32(session.TxBytes >> 32); txGiga > 0 {
		packet.Add(53, encodeUint32(txGiga))
	}
	packet.Add(47, encodeUint32(uint32(session.RxPackets)))
	packet.Add(48, encodeUint32(uint32(session.TxPackets)))

	if session.Attributes != nil {
		if v, ok := session.Attributes[aaa.AttrIPv4Address]; ok {
			if ip := net.ParseIP(v); ip != nil {
				packet.Add(8, radius.Attribute(ip.To4()))
			}
		}
		if v, ok := session.Attributes[aaa.AttrIPv6WANPrefix]; ok {
			if encoded := encodeIPv6Prefix(v); encoded != nil {
				packet.Add(97, encoded)
			}
		}
		if v, ok := session.Attributes[aaa.AttrIPv6Prefix]; ok {
			if encoded := encodeIPv6Prefix(v); encoded != nil {
				packet.Add(123, encoded)
			}
		}
	}

	now := uint32(time.Now().Unix())
	packet.Add(55, encodeUint32(now))

	resp, rc, err := p.sendAcctWithFailover(packet)
	if err != nil {
		return err
	}

	if resp.Code != radius.CodeAccountingResponse {
		p.radiusStats.IncrAcctError(rc.addr, fmt.Errorf("unexpected code %d", resp.Code))
		return fmt.Errorf("unexpected accounting response code: %d", resp.Code)
	}

	p.radiusStats.IncrAcctResponse(rc.addr)
	return nil
}
