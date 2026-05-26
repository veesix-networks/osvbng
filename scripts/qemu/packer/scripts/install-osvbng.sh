#!/bin/bash
set -e

groupadd -r osvbng || true

mkdir -p /usr/local/bin /etc/osvbng /etc/osvbng/release-keys /var/log/osvbng /var/lib/osvbng /run/osvbng /usr/lib/x86_64-linux-gnu/vpp_plugins /usr/share/osvbng/templates

# Upgrade-pipeline state lives outside /var/lib so a future osvbng .deb
# postrm cannot accidentally wipe rollback snapshots. /var/opt is the
# FHS-reserved location for add-on package variable data and is not
# touched by apt operations during base-system upgrades.
mkdir -p /var/opt/osvbng/rollback /var/opt/osvbng/quarantine

mv /tmp/osvbngd /usr/local/bin/osvbngd
mv /tmp/osvbngcli /usr/local/bin/osvbngcli
mv /tmp/vpp-plugins/*.so /usr/lib/x86_64-linux-gnu/vpp_plugins/
cp -r /tmp/templates/* /usr/share/osvbng/templates/

# Release-signing public key. Staged at /tmp/cosign.pub by the packer
# `file` provisioner that reads from release-keys/cosign.pub at the
# repo root. Failure to stage means the image would have no trust
# anchor and the upgrade flow would refuse every tarball — better to
# fail the build loudly than ship a silently broken image.
if [ ! -f /tmp/cosign.pub ]; then
    echo "FATAL: /tmp/cosign.pub missing. The QEMU build requires release-keys/cosign.pub." >&2
    echo "       Generate one with: scripts/release/generate-signing-key.sh" >&2
    exit 1
fi
mv /tmp/cosign.pub /etc/osvbng/release-keys/cosign.pub
chmod 0644 /etc/osvbng/release-keys/cosign.pub

chmod +x /usr/local/bin/osvbngd /usr/local/bin/osvbngcli

chown -R root:osvbng /etc/osvbng /var/log/osvbng /var/lib/osvbng /run/osvbng /var/opt/osvbng
chmod 0750 /var/opt/osvbng /var/opt/osvbng/rollback /var/opt/osvbng/quarantine

echo 'd /run/osvbng 0750 root osvbng -' > /etc/tmpfiles.d/osvbng.conf
