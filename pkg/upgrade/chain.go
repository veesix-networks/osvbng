// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package upgrade

import "context"

// PrunePolicy controls how rollback snapshots are retained between
// ApplyOne calls. Single-tarball Apply uses PruneKeepN with N=1; a
// chain coordinator overrides to PruneSuppress for the duration of a
// multi-step chain so a mid-chain failure can still roll back to the
// original baseline.
type PrunePolicy int

const (
	// PruneKeepN keeps the N most recent snapshots and removes older
	// ones after a successful commit. Default for single-tarball use.
	PruneKeepN PrunePolicy = iota

	// PruneSuppress disables pruning for this ApplyOne call. Caller is
	// responsible for pruning after the multi-step operation completes
	// (or remembering that the rollback chain has grown).
	PruneSuppress
)

// ApplyOptions tunes a single ApplyOne call. Zero-value defaults are
// correct for the single-tarball operator UX.
type ApplyOptions struct {
	// PrunePolicy controls snapshot retention after commit. Zero
	// value (PruneKeepN) is correct for single-tarball Apply.
	PrunePolicy PrunePolicy

	// ExpectedFrom, if non-empty, asserts the on-disk current version
	// equals this string before apply. Used by a chain coordinator to
	// validate stepwise progression: each step's ExpectedFrom equals
	// the previous step's To value. Single-tarball Apply leaves it
	// empty so the runtime-discovered version is accepted.
	ExpectedFrom string

	// KeepN is the snapshot retention count when PrunePolicy is
	// PruneKeepN. Zero defaults to 1 (keep N-1).
	KeepN int
}

// ApplyResult is what a successful ApplyOne returns. Carries explicit
// {From, To} so a chain coordinator can validate stepwise progression
// without reading pkg/version.Version from the running osvbngcli
// process (which is stale after the apply has replaced osvbngcli on
// disk).
type ApplyResult struct {
	From            string
	To              string
	SnapshotDir     string
	ArtifactsSwap   []string
	HealthOutcome   string
	JournalEndPhase string
}

// ChainCoordinator is the seam #114 (install + catch-up workflow) will
// implement. Tier A defines the interface here so #114 has a stable
// contract to code against; Tier A does NOT ship a coordinator
// implementation — the operator UX in Tier A exposes only
// single-tarball Apply.
type ChainCoordinator interface {
	// Apply runs a sequence of tarballs in order with version-chain
	// validation between steps and a chain-wide prune policy. Returns
	// one ApplyResult per successful step. On failure, the returned
	// slice covers all steps that succeeded; the caller decides
	// whether the chain unwinds or stops at the failure point.
	Apply(ctx context.Context, tarballs []string) ([]*ApplyResult, error)
}
