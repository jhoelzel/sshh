#!/bin/sh
set -u

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
report=${1:-"$root/docs/benchmarks/m1-macos-arm64.json"}

if [ "$(uname -s)" != "Darwin" ]; then
  printf '%s\n' 'The packaged WKWebView terminal benchmark requires macOS.' >&2
  exit 1
fi

cd "$root" || exit 1
VITE_TERMINAL_BENCHMARK=1 make build || exit 1

status=0
go run ./cmd/terminalbench -app "$root/build/bin/shh-h.app/Contents/MacOS/shhh" -report "$report" || status=$?

# Restore the ordinary product bundle even when a benchmark budget fails.
make build || exit 1
exit "$status"
