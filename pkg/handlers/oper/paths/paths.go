// Copyright 2025 Veesix Networks Ltd
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package paths

import (
	"github.com/veesix-networks/osvbng/pkg/paths"
)

type Path string

const (
	SystemLoggingLevel     Path = "system.logging.level.<*>"
	SystemEventsDebug      Path = "system.events.debug"
	SubscriberSessionClear Path = "subscriber.session.clear"

	HASwitchover Path = "ha.switchover"

	CGNATTestMapping Path = "cgnat.test-mapping"
)

func (p Path) String() string {
	return string(p)
}

func (p Path) ExtractWildcards(path string, expectedCount int) ([]string, error) {
	return paths.Extract(path, string(p))
}

func Build(pattern Path, values ...string) (string, error) {
	return paths.Build(string(pattern), values...)
}

func Extract(path string, pattern Path) ([]string, error) {
	return paths.Extract(path, string(pattern))
}
