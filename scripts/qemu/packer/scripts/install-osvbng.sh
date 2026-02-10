#!/bin/bash
set -e

groupadd -r osvbng || true

mkdir -p /usr/local/bin /etc/osvbng /var/log/osvbng /var/lib/osvbng /run/osvbng /usr/lib/x86_64-linux-gnu/vpp_plugins /usr/share/osvbng/templates

mv /tmp/osvbngd /usr/local/bin/osvbngd
mv /tmp/osvbngcli /usr/local/bin/osvbngcli
mv /tmp/vpp-plugins/*.so /usr/lib/x86_64-linux-gnu/vpp_plugins/
cp -r /tmp/templates/* /usr/share/osvbng/templates/

chmod +x /usr/local/bin/osvbngd /usr/local/bin/osvbngcli

chown -R root:osvbng /etc/osvbng /var/log/osvbng /var/lib/osvbng /run/osvbng

echo 'd /run/osvbng 0750 root osvbng -' > /etc/tmpfiles.d/osvbng.conf
