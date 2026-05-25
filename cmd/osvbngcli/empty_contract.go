// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package main

// NewEmptyContract returns a typed empty Contract used as the placeholder
// value while the northbound API is unreachable. Holding a non-nil value
// lets every consumer that ranges Commands or calls matchCommand operate
// without nil-guarding: the range yields zero iterations, matchCommand
// returns "unrecognized command", and the REPL surfaces a clean error
// instead of a panic.
func NewEmptyContract() *Contract {
	return &Contract{
		Commands: []*GeneratedCommand{},
	}
}
