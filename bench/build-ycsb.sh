#!/usr/bin/env bash
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
src="$(mktemp -d)/go-ycsb"

git clone --depth 1 --branch v1.0.3 https://github.com/pingcap/go-ycsb.git "$src"
cp -r "$here/kvstore" "$src/db/kvstore"
printf 'package main\n\nimport _ "github.com/pingcap/go-ycsb/db/kvstore"\n' > "$src/cmd/go-ycsb/kvstore.go"

(cd "$src" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$here/../go-ycsb" ./cmd/go-ycsb)
echo "built ./go-ycsb"
