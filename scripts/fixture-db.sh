#!/bin/sh
# fixture-db.sh <tag>: writes internal/store/testdata/upgrade/<tag>.db.gz with
# the store code of <tag>, for TestUpgradeFixtures.
set -eu
tag=$1
root=$(git rev-parse --show-toplevel)
wt=$(mktemp -d)
trap 'git -C "$root" worktree remove --force "$wt" >/dev/null 2>&1 || true' EXIT
git -C "$root" worktree add --detach "$wt" "$tag" >/dev/null
mkdir -p "$wt/cmd/fixturewriter"
cp "$root/scripts/fixturewriter/main.go.txt" "$wt/cmd/fixturewriter/main.go"
out="$root/internal/store/testdata/upgrade/$tag.db"
mkdir -p "$(dirname "$out")"
rm -f "$out" "$out-wal" "$out-shm"
(cd "$wt" && CGO_ENABLED=0 go run ./cmd/fixturewriter "$out")
# fold the WAL into the main file so one file is committed
sqlite3 "$out" 'PRAGMA wal_checkpoint(TRUNCATE); PRAGMA journal_mode=DELETE; VACUUM;' >/dev/null
rm -f "$out-wal" "$out-shm"
gzip -9nf "$out"
echo "wrote $out.gz"
