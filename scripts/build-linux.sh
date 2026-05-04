#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

OUT="${OUT:-./dist/faceless-server}"
mkdir -p "$(dirname "$OUT")"

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
  -ldflags="-s -w -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
  -o "$OUT" ./cmd/server

echo "built: $OUT ($(du -h "$OUT" | cut -f1))"
