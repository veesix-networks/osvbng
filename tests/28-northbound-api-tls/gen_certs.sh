#!/usr/bin/env bash
# Copyright 2026 The osvbng Authors
# Licensed under the GNU General Public License v3.0 or later.
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Generate a self-signed CA, server cert, and client cert for the
# 28-northbound-api-tls Robot suite. Idempotent.

set -euo pipefail

CERT_DIR="$(cd "$(dirname "$0")" && pwd)/config/certs"
mkdir -p "$CERT_DIR"
cd "$CERT_DIR"

cat >openssl-server.cnf <<'EOF'
[req]
distinguished_name = dn
prompt = no
req_extensions = v3_req

[dn]
CN = bng1

[v3_req]
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = DNS:bng1,DNS:localhost,IP:127.0.0.1
EOF

cat >openssl-client.cnf <<'EOF'
[req]
distinguished_name = dn
prompt = no

[dn]
CN = osvbng-test-client
EOF

openssl ecparam -name prime256v1 -genkey -noout -out ca.key
openssl req -x509 -new -key ca.key -sha256 -days 365 -subj "/CN=osvbng-test-ca" -out ca.crt

openssl ecparam -name prime256v1 -genkey -noout -out server.key
openssl req -new -key server.key -config openssl-server.cnf -out server.csr
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out server.crt -days 365 -sha256 \
    -extensions v3_req -extfile openssl-server.cnf

openssl ecparam -name prime256v1 -genkey -noout -out client.key
openssl req -new -key client.key -config openssl-client.cnf -out client.csr
openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
    -out client.crt -days 365 -sha256

rm -f openssl-server.cnf openssl-client.cnf server.csr client.csr ca.srl
chmod 0644 *.crt
chmod 0600 *.key
