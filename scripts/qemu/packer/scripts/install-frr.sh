#!/bin/bash
set -e

export DEBIAN_FRONTEND=noninteractive

curl -s https://deb.frrouting.org/frr/keys.gpg | tee /usr/share/keyrings/frrouting.gpg > /dev/null
echo "deb [signed-by=/usr/share/keyrings/frrouting.gpg] https://deb.frrouting.org/frr $(lsb_release -s -c) frr-stable" | tee /etc/apt/sources.list.d/frr.list

apt-get update && apt-get install -y --no-install-recommends \
    frr \
    frr-pythontools \
    && rm -rf /var/lib/apt/lists/*

systemctl disable frr
systemctl stop frr || true
