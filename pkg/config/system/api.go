// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package system

import (
	"github.com/veesix-networks/osvbng/pkg/netbind"
)

type APIConfig struct {
	netbind.EndpointBinding `json:",inline" yaml:",inline"`

	Address string `json:"address,omitempty" yaml:"address,omitempty"`
}
