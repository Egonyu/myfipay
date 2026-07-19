#!/usr/bin/env bash
# Run the Go test suite the same way everywhere: dockerized toolchain, no Go
# needed on the host. Memory-capped and single-threaded so it survives the
# 1GB droplet (compile gets OOM-killed otherwise).
#
# Usage: scripts/test.sh [package-path ...]   (default: ./...)
set -euo pipefail
cd "$(dirname "$0")/../api"

PKGS=("${@:-./...}")

docker run --rm -m 700m \
  -v "$PWD":/src \
  -v myfibase_gomod:/go/pkg/mod \
  -v myfibase_gobuild:/root/.cache/go-build \
  -w /src -e GOMAXPROCS=1 -e GOFLAGS=-p=1 \
  golang:1.25 sh -c "go vet ${PKGS[*]} && go test ${PKGS[*]} -count=1"
