// Copyright 2026 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package radius

import (
	"context"
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
	packet.Add(47, encodeUint32(uint32(session.RxPackets)))
	packet.Add(48, encodeUint32(uint32(session.TxPackets)))

	if session.Attributes != nil {
		if v, ok := session.Attributes[aaa.AttrIPv4Address]; ok {
			if ip := net.ParseIP(v); ip != nil {
				packet.Add(8, radius.Attribute(ip.To4()))
			}
		}
	}

	now := uint32(time.Now().Unix())
	packet.Add(55, encodeUint32(now))

	resp, rc, err := p.sendWithFailover(packet, p.acctConns)
	if err != nil {
		return err
	}

	if resp.Code == radius.CodeAccountingResponse {
		p.radiusStats.IncrAcctResponse(rc.addr)
	} else {
		p.radiusStats.IncrAcctError(rc.addr, nil)
	}

	return nil
}
