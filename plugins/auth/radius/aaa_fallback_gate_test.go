// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package radius

import (
	"context"
	"testing"

	"github.com/veesix-networks/osvbng/pkg/auth"
	"github.com/veesix-networks/osvbng/pkg/logger"
)

// When the configured policy username could not be resolved the protocol layer
// hands RADIUS the MAC fallback with UsernameFallback set. RADIUS must gate the
// subscriber rather than authenticate a misrepresented identity, and it must do
// so without contacting a server (no authConns are wired here).
func TestAuthenticateGatesOnUsernameFallback(t *testing.T) {
	p := &Provider{logger: logger.NewTest()}

	resp, err := p.Authenticate(context.Background(), &auth.AuthRequest{
		Username:         "aa:42:a1:0a:54:97",
		MAC:              "aa:42:a1:0a:54:97",
		PolicyName:       "p1",
		UsernameFallback: true,
	})
	if err != nil {
		t.Fatalf("gate must not error, got %v", err)
	}
	if resp == nil || resp.Allowed {
		t.Fatalf("RADIUS must reject when the configured identity is unresolved")
	}
}
