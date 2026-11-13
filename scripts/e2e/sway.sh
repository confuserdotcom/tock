#!/bin/sh
# Real headless Sway session for the e2e test: wlroots' headless backend,
# two foot terminals with known titles. Runs on ubuntu-latest.
set -eu
export XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR:-$(mktemp -d)}
chmod 700 "$XDG_RUNTIME_DIR"
export WLR_BACKENDS=headless WLR_RENDERER=pixman WLR_LIBINPUT_NO_DEVICES=1
printf 'output HEADLESS-1 resolution 1280x800\n' > "$XDG_RUNTIME_DIR/sway.conf"
sway -c "$XDG_RUNTIME_DIR/sway.conf" &
for _ in 1 2 3 4 5 6 7 8 9 10; do
	SWAYSOCK=$(ls "$XDG_RUNTIME_DIR"/sway-ipc.*.sock 2>/dev/null | head -1) && [ -n "$SWAYSOCK" ] && break
	sleep 1
done
export SWAYSOCK
unset DISPLAY
# the socket appears before sway answers on it
for _ in 1 2 3 4 5 6 7 8 9 10; do
	swaymsg -t get_version >/dev/null 2>&1 && break
	sleep 1
done
swaymsg -q exec 'foot -T tock-e2e-alpha sleep 600'
swaymsg -q exec 'foot -T tock-e2e-beta sleep 600'
sleep 3
swaymsg -t get_tree | grep -c tock-e2e || true
export E2E_TITLES=tock-e2e-alpha,tock-e2e-beta
export E2E_FOCUS='swaymsg -q "[title=\"^{title}$\"] focus"'
export E2E_DESKTOP=sway
go test -tags e2e ./e2e -v -count=1
