#!/bin/sh
set -u

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mode=burst
if [ "${1:-}" = "--soak" ]; then
  mode=soak
  shift
elif [ "${1:-}" = "--lifecycle" ]; then
  mode=lifecycle
  shift
fi
case "$mode" in
  burst) default_report="$root/docs/benchmarks/m1-macos-arm64.json" ;;
  soak) default_report="$root/docs/benchmarks/m1-macos-arm64-soak.json" ;;
  lifecycle) default_report="${TMPDIR:-/tmp}/m3-macos-arm64-lifecycle.json" ;;
  *) printf 'Unsupported terminal benchmark mode: %s\n' "$mode" >&2; exit 2 ;;
esac
report=${1:-"$default_report"}

if [ "$(uname -s)" != "Darwin" ]; then
  printf '%s\n' 'The packaged WKWebView terminal benchmark requires macOS.' >&2
  exit 1
fi

cd "$root" || exit 1
restore_needed=1
restore_product_build() {
  code=$?
  trap - EXIT
  if [ "$restore_needed" -eq 1 ]; then
    make build || code=1
  fi
  exit "$code"
}
trap restore_product_build EXIT

VITE_TERMINAL_BENCHMARK=1 make build || exit 1
status=0
go run ./cmd/terminalbench -mode "$mode" -app "$root/build/bin/shh-h.app/Contents/MacOS/shhh" -report "$report" || status=$?

# Restore the ordinary product bundle even when a benchmark budget fails.
if make build; then
  restore_needed=0
else
  status=1
  restore_needed=0
fi
trap - EXIT
exit "$status"
