#!/usr/bin/env bash
# Lock-agnostic end-to-end harness: build trackd, run it against a temp data
# dir for SECONDS, stop it, and print the temp dir as the last stdout line.
# Usage: scripts/smoke.sh SECONDS [term|kill9]
set -euo pipefail

cd "$(dirname "$0")/.."

SECS="${1:?usage: scripts/smoke.sh SECONDS [term|kill9]}"
MODE="${2:-term}"

go build -o bin/trackd ./cmd/trackd

D=$(mktemp -d "${TMPDIR:-/tmp}/tock-smoke.XXXXXX")
chmod 700 "$D"

TOCK_DATA_DIR="$D" TOCK_STATE_DIR="$D" bin/trackd >"$D/trackd.out" 2>&1 &
pid=$!
sleep "$SECS"

rc=0
case "$MODE" in
  term)
    kill -TERM "$pid"
    wait "$pid" || rc=$?
    ;;
  kill9)
    kill -KILL "$pid"
    wait "$pid" 2>/dev/null || true
    ;;
  *)
    kill -KILL "$pid"
    wait "$pid" 2>/dev/null || true
    echo "unknown mode: $MODE" >&2
    exit 2
    ;;
esac

echo "$D"
exit "$rc"
