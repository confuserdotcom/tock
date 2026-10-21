#!/bin/sh
# Run by the post-commit and post-merge hooks that make hooks installs.
# On main it starts make update in the background and returns at once, so git
# never waits for a build. Output goes to update.log in the state dir.
#
#   git-hook.sh              hook mode
#   git-hook.sh run ROOT STATE   what the hook starts (not for you to run)

if [ "$1" != run ]; then
	[ "$(git symbolic-ref --short -q HEAD)" = main ] || exit 0
	root=$(git rev-parse --show-toplevel) || exit 0
	# A post-commit hook can inherit a temporary index that would corrupt
	# git worktree add.
	unset GIT_DIR GIT_INDEX_FILE GIT_WORK_TREE GIT_PREFIX
	state=${TOCK_STATE_DIR:-$HOME/.local/state/tock}
	mkdir -p "$state"
	log=$state/update.log
	if [ -f "$log" ] && [ "$(wc -c <"$log")" -gt 1048576 ]; then
		mv -f "$log" "$log.1"
	fi
	self=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")
	nohup sh "$self" run "$root" "$state" >>"$log" 2>&1 </dev/null &
	exit 0
fi

root=$2
state=$3

# One update at a time. A run that finds the lock held leaves: the holder
# checks main again before it stops.
lock=$state/update.lock
if ! mkdir "$lock" 2>/dev/null; then
	pid=$(cat "$lock/pid" 2>/dev/null)
	if [ -z "$pid" ]; then
		sleep 1 # the holder has not written its pid yet
		pid=$(cat "$lock/pid" 2>/dev/null)
	fi
	if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
		exit 0
	fi
	rm -rf "$lock"
	mkdir "$lock" 2>/dev/null || exit 0
fi
echo $$ >"$lock/pid"
trap 'rm -rf "$lock"' EXIT
trap 'exit 1' HUP INT TERM

PATH="$PATH:/usr/local/go/bin:/opt/homebrew/bin"
export PATH
cd "$root" || exit 1

# Commits that land during a build make another pass. The cap stops a build
# that never records last-update from looping for ever.
i=0
while [ "$i" -lt 5 ]; do
	i=$((i + 1))
	echo "== $(date) make update"
	make -C "$root" update TOCK_STATE="$state" || break
	v=$(git describe --always main 2>/dev/null)
	[ -n "$v" ] && [ "$v" = "$(cat "$state/last-update" 2>/dev/null)" ] && break
done
