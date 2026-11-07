#!/bin/bash
# Records the notch for the README and launch videos: the showcase dashboard
# full screen, `tockbar notch-demo` playing its moments over it, the screen
# captured at 60 fps. Takes about 80 seconds; don't touch the Mac meanwhile.
# The live menu bar app is stopped for the recording and started again after.
set -euo pipefail
here="$(cd "$(dirname "$0")/.." && pwd)"
out="$here/footage/notch"
bar="$HOME/.local/share/tock/tock.app/Contents/MacOS/tockbar"
plist="$HOME/Library/LaunchAgents/com.tock.bar.plist"
secs="${SECS:-66}"
mkdir -p "$out"
pids=()
restore() {
  for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done
  pkill -f "tockbar notch-demo" 2>/dev/null || true
  launchctl bootstrap "gui/$(id -u)" "$plist" 2>/dev/null || launchctl kickstart -k "gui/$(id -u)/com.tock.bar" 2>/dev/null || true
  echo "menu bar app restarted"
}
trap restore EXIT

[ -x "$here/footage/showcase.test" ] || (cd "$here/.." && go test -c -tags demo -o "$here/footage/showcase.test" ./internal/web)
TOCK_DEMO_PORT=7799 "$here/footage/showcase.test" -test.run '^TestShowcase$' -test.timeout 0 >/dev/null 2>&1 & pids+=($!)
sleep 2
launchctl bootout "gui/$(id -u)/com.tock.bar" 2>/dev/null || true
node "$here/capture/backdrop.mjs" & pids+=($!)
sleep 5   # kiosk window settles into full screen
"$bar" notch-demo >/dev/null 2>&1 & pids+=($!)
sleep 1.5
echo "recording ${secs}s — hands off"
ffmpeg -y -loglevel error -f avfoundation -capture_cursor 0 -framerate 60 -pixel_format nv12 -i "Capture screen 0:none" \
  -t "$secs" -c:v libx264 -preset veryfast -crf 12 -pix_fmt yuv420p "$out/screen.mp4"
echo "saved $out/screen.mp4"
