#!/bin/sh
# Runs gofmt, go vet and go test inside a golang container, for machines
# without a local Go toolchain.
set -e
src="$(cd "$(dirname "$0")/.." && (pwd -W 2>/dev/null || pwd))"
MSYS_NO_PATHCONV=1 docker run --rm -v "$src:/src" -v gpu-governor-gomod:/go/pkg -w /src golang:1.23-alpine \
  sh -c 'test -z "$(gofmt -l .)" || { gofmt -l .; exit 1; }; go vet ./... && go test ./... "$@"' -- "$@"
