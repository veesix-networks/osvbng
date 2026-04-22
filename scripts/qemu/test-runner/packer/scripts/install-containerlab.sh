#!/bin/bash
# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

bash -c "$(curl -sL https://get.containerlab.dev)" -- -v "${CONTAINERLAB_VERSION}"
