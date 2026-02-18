#!/bin/bash
set -euo pipefail

API="${API:-http://172.30.0.2:8080}"
COUNT="${1:-10}"
MAC_START="${2:-1}"

for i in $(seq "$MAC_START" $((MAC_START + COUNT - 1))); do
    mac=$(printf "02:00:00:00:00:%02x" "$i")

    resp=$(curl -sf -X POST "$API/api/exec/subscriber/auth/local/users/create" \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$mac\",\"enabled\":true}")

    user_id=$(echo "$resp" | jq -r '.user_id')
    echo "Created user $mac (id=$user_id)"
done
