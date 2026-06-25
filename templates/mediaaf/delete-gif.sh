#!/bin/bash
set -euo pipefail

if [ $# -eq 0 ]; then
  echo "Usage: $0 <gif_id> [gif_id ...]"
  exit 1
fi

ANTFLY_URL="${ANTFLY_URL:-http://localhost:8080/api/v1}"
TABLE="${TABLE:-honeycomb}"

# Build JSON array of IDs
deletes=$(printf '%s\n' "$@" | jq -R . | jq -s .)

curl -X POST "${ANTFLY_URL}/tables/${TABLE}/batch" \
  -H "Content-Type: application/json" \
  -d "{\"deletes\": ${deletes}}"
