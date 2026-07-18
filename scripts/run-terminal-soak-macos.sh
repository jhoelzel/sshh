#!/bin/sh
set -u

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
exec "$root/scripts/run-terminal-benchmark-macos.sh" --soak "$@"
