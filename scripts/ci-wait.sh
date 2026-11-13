#!/bin/sh
# ci-wait.sh [REF]: wait for the ci and e2e workflows on REF's commit
# (default HEAD) and exit 0 only when both passed. Push the branch first;
# fast-forward main only after this succeeds.
#
#   git push -u origin my-branch && scripts/ci-wait.sh && \
#     git checkout main && git merge --ff-only my-branch && git push origin main
#
# Needs the GitHub CLI (gh), logged in.

set -eu
sha=$(git rev-parse "${1:-HEAD}")
short=$(printf %.7s "$sha")
deadline=$(($(date +%s) + 3600))

for wf in ci.yml e2e.yml; do
	while :; do
		line=$(gh run list --workflow "$wf" --commit "$sha" --limit 1 \
			--json status,conclusion,databaseId \
			--jq '.[0] | "\(.status) \(.conclusion) \(.databaseId)"' 2>/dev/null || true)
		set -- $line
		status=${1:-none} conclusion=${2:-} id=${3:-}
		if [ "$status" = completed ]; then
			if [ "$conclusion" = success ]; then
				echo "$wf on $short: passed"
				break
			fi
			echo "$wf on $short: $conclusion (gh run view $id --log-failed)" >&2
			exit 1
		fi
		if [ "$(date +%s)" -gt "$deadline" ]; then
			echo "$wf on $short: still $status after an hour" >&2
			exit 1
		fi
		sleep 30
	done
done
echo "$short is green; safe to fast-forward main"
