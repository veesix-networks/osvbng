# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later

VENDOR=osvbng
NAME=osvbng
IMAGE_FORMAT=qcow2
IMAGE_GLOB=*.qcow2

# Packer produces osvbng-debian12.qcow2 (gunzipped from the .gz the
# build artifact ships as). Extract the upstream osvbng semver from the
# filename: osvbng-debian12-v0.13.0.qcow2 -> 0.13.0. If the operator
# drops a generic osvbng-debian12.qcow2 (no version baked into the
# name), VERSION will be empty and the override below stamps "local".
VERSION=$(shell echo $(IMAGE) | sed -ne 's/^osvbng-debian12-v\([0-9]\+\.[0-9]\+\.[0-9]\+\)\.qcow2$$/\1/p')
ifeq ($(VERSION),)
VERSION=local
endif

-include ../makefile-sanity.include
-include ../makefile.include

# Override the upstream `vrnetlab/vr-osvbng:VERSION` tag with the
# project's container-tag convention. Same image repo as the Docker
# bng container; the `ci-` tag prefix discriminates the QEMU-wrapped
# build from the Docker-native one.
TAG_NAME = veesixnetworks/osvbng:ci-v$(VERSION)
