#!/usr/bin/env bash
# Cross-compile coordinator and runner for ARM64 (build-vm).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="$ROOT/bin/arm64"
mkdir -p "$OUT"

cd "$ROOT"
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=arm64

echo "Building tinycloud-build-coordinator (arm64)..."
go build -ldflags="-w -s" -o "$OUT/tinycloud-build-coordinator" ./cmd/build-coordinator

echo "Building tinycloud-build-runner (arm64)..."
go build -ldflags="-w -s" -o "$OUT/tinycloud-build-runner" ./cmd/build-runner

ls -la "$OUT"
echo "Done. Copy to build-vm and run scripts/deploy/bootstrap-build-vm.sh"
