#!/usr/bin/env bash
set -euo pipefail

go run ./cmd/server &
api_pid=$!

cleanup() {
  kill "$api_pid" 2>/dev/null || true
  wait "$api_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

pnpm --dir web dev
