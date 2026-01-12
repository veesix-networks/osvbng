#!/bin/bash
set -e

apt-get clean
rm -rf /var/lib/apt/lists/*
rm -rf /tmp/*
rm -rf /var/tmp/*

dd if=/dev/zero of=/EMPTY bs=1M || true
rm -f /EMPTY

sync
