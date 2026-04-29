// Copyright 2026 The osvbng Authors
// Licensed under the GNU General Public License v3.0 or later.
// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import "errors"

var (
	ErrUnboundedLabel = errors.New("telemetry: label is in the unbounded label list (set StreamingOnly=true to override)")
	ErrTypeMismatch   = errors.New("telemetry: metric already registered with a different type")
	ErrSchemaMismatch = errors.New("telemetry: metric already registered with a different label schema")
	ErrInvalidLabel   = errors.New("telemetry: invalid label name")
	ErrLabelCount     = errors.New("telemetry: label values count does not match metric label schema")
	ErrInvalidMetric  = errors.New("telemetry: metric options invalid")
)
