// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

// MatchGlob reports whether path matches a simple glob pattern.
// The wildcard '*' matches any sequence of characters including the empty
// string. Empty pattern and "*" both match every path.
func MatchGlob(pattern, path string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	return matchGlob(pattern, path)
}

func matchGlob(pattern, key string) bool {
	i, j := 0, 0
	for i < len(pattern) && j < len(key) {
		if pattern[i] == '*' {
			if i == len(pattern)-1 {
				return true
			}
			for j < len(key) {
				if matchGlob(pattern[i+1:], key[j:]) {
					return true
				}
				j++
			}
			return false
		}
		if pattern[i] != key[j] {
			return false
		}
		i++
		j++
	}
	return i == len(pattern) && j == len(key)
}
