// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package ospf

import "encoding/json"

type BorderRouterMap struct {
	Routers map[string]json.RawMessage `json:"routers"`
}
