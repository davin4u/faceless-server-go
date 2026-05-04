#!/usr/bin/env bash
# Parity smoke test: hit the same REST endpoints on Node (:3001) and Go (:3002)
# servers and diff the JSON responses.
#
# Pre-req: start the Node server with PORT=3001 and the Go server with PORT=3002.
set -euo pipefail

NODE_BASE="${NODE_BASE:-http://localhost:3001}"
GO_BASE="${GO_BASE:-http://localhost:3002}"

normalize() { jq -S '
  walk(if type == "object" then
        (if has("challenge") then .challenge = "<challenge>" else . end)
      | (if has("contactCode") then .contactCode = "<code>" else . end)
      | (if has("id") then .id = "<id>" else . end)
      | (if has("name") then .name = "<name>" else . end)
      | (if has("displayName") then .displayName = "<name>" else . end)
      | (if has("uptimeSeconds") then .uptimeSeconds = 0 else . end)
      | (if has("nodeVersion") then .nodeVersion = "<runtime>" else . end)
      | (if has("ramUsedMb") then .ramUsedMb = 0 else . end)
      | (if has("ramTotalMb") then .ramTotalMb = 0 else . end)
      | (if has("cpuLoadAvg") then .cpuLoadAvg = [] else . end)
      | (if has("diskUsedGb") then .diskUsedGb = 0 else . end)
      | (if has("diskTotalGb") then .diskTotalGb = 0 else . end)
      else . end)' ; }

check_path() {
  local method="$1" path="$2" body="${3:-}"
  echo "=== $method $path ==="
  local n g
  if [[ "$method" == GET ]]; then
    n=$(curl -s "$NODE_BASE$path" | normalize)
    g=$(curl -s "$GO_BASE$path"   | normalize)
  else
    n=$(curl -s -X "$method" "$NODE_BASE$path" -H 'Content-Type: application/json' -d "$body" | normalize)
    g=$(curl -s -X "$method" "$GO_BASE$path"   -H 'Content-Type: application/json' -d "$body" | normalize)
  fi
  if [[ "$n" == "$g" ]]; then
    echo "OK"
  else
    echo "DIFFERENT"
    diff <(echo "$n") <(echo "$g") || true
  fi
}

check_path GET  /health
check_path POST /api/pow/challenge '{"action":"register"}'
check_path POST /api/generate-name '{}'
check_path POST /api/recover       '{"publicKey":"missing-pub-key"}'
