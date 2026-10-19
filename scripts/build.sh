#!/usr/bin/env sh
set -eu
pnpm install --frozen-lockfile
pnpm run build:web
go test ./...
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -o bin/uniproxy ./cmd/uniproxy
CGO_ENABLED=0 go build -trimpath -o bin/core-install ./cmd/core-install
