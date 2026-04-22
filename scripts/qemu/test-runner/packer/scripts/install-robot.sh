#!/bin/bash
# Copyright 2025 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

set -euo pipefail

apt-get update
apt-get install -y python3 python3-pip python3-venv

pip3 install --break-system-packages robotframework==7.4.1
