#!/bin/sh
# trackd-usage.sh [MINUTES]: measure the running trackd's CPU and memory.
#
# CPU is the process's CPU time used over the window divided by the
# window's wall time, so short spikes count at their true weight. Memory
# is the macOS footprint (what Activity Monitor shows), sampled at the
# start and the end; RSS overstates it because it counts shared pages.
# Leave the Mac as it normally is while it runs: the number is only as
# typical as the window.

set -eu
mins=${1:-10}
pid=$(pgrep -x trackd | head -n 1 || true)
[ -n "$pid" ] || { echo "trackd is not running" >&2; exit 1; }

# cputime as seconds, from ps's [[dd-]hh:]mm:ss.cc
cpu() {
	ps -o time= -p "$pid" | awk '{
		n = split($1, d, "-"); days = (n == 2) ? d[1] : 0; t = d[n]
		k = split(t, p, ":"); s = 0
		for (i = 1; i <= k; i++) s = s * 60 + p[i]
		printf "%.2f\n", s + days * 86400 }'
}
footprint_mb() {
	if command -v footprint >/dev/null 2>&1; then
		# "trackd [123]: 64-bit    Footprint: 42 MB (16384 bytes per page)"
		footprint "$pid" 2>/dev/null | awk '{ for (i = 1; i < NF - 1; i++) if ($i == "Footprint:") {
			v = $(i + 1); u = $(i + 2)
			if (u == "KB") v /= 1024; if (u == "GB") v *= 1024
			printf "%.0f\n", v; exit } }'
	else
		ps -o rss= -p "$pid" | awk '{ printf "%.0f\n", $1 / 1024 }'
	fi
}

c0=$(cpu); m0=$(footprint_mb); t0=$(date +%s)
echo "trackd pid $pid: measuring for $mins min (footprint now ${m0} MB)"
sleep $((mins * 60))
c1=$(cpu); m1=$(footprint_mb); t1=$(date +%s)
kill -0 "$pid" 2>/dev/null || { echo "trackd restarted during the window" >&2; exit 1; }

awk -v c0="$c0" -v c1="$c1" -v t0="$t0" -v t1="$t1" -v m0="$m0" -v m1="$m1" 'BEGIN {
	printf "cpu        %.2f %% (%.2f s over %d s)\n", (c1 - c0) * 100 / (t1 - t0), c1 - c0, t1 - t0
	printf "footprint  %s MB at start, %s MB at end\n", m0, m1 }'
