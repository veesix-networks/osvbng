// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package configmgr

import (
	"encoding/json"

	"github.com/veesix-networks/osvbng/pkg/config"
)

// DecodeCandidatePluginConfig extracts a typed plugin config from a
// candidate session's *config.Config (i.e. cfg.Plugins[namespace]).
// Used by post-vrfmgr validators to access the candidate's plugin
// config before it has been applied to the global plugin state.
//
// Returns (nil, nil) when the namespace is not present in cfg.Plugins.
func DecodeCandidatePluginConfig[T any](cfg *config.Config, namespace string) (*T, error) {
	if cfg == nil || cfg.Plugins == nil {
		return nil, nil
	}
	raw, ok := cfg.Plugins[namespace]
	if !ok {
		return nil, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
