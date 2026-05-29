// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package cgnat

import (
	"context"

	"github.com/veesix-networks/osvbng/pkg/models"
)

// SessionProvider is the thin contract CGNAT consumes for restored-session
// lookups. The subscriber component owns session storage and satisfies this
// interface; CGNAT does not duplicate that ownership.
//
// SessionSnapshot returns the typed subscriber session for the given ID with
// AccessType-aware decoding so PPPoE / L2TP entries are not silently
// mis-decoded as IPoE. GetSessions returns all active sessions; CGNAT uses it
// for the post-restore scan that recovers deterministic and bypass
// subscribers (which have no per-subscriber opdb record).
type SessionProvider interface {
	SessionSnapshot(ctx context.Context, sessionID string) (models.SubscriberSession, bool)
	GetSessions(ctx context.Context, accessType, protocol string, svlan uint32) ([]models.SubscriberSession, error)
}
