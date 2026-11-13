#!/bin/sh
# Real X11 session for the e2e test: Xvfb, a window manager ($1: openbox
# or i3) and two xterms with known titles. Runs on ubuntu-latest.
set -eu
wm=${1:-openbox}
export DISPLAY=:99
Xvfb :99 -screen 0 1280x800x24 &
sleep 2
case $wm in
openbox)
	openbox &
	export E2E_FOCUS='xdotool search --name "^{title}$" windowactivate --sync'
	export E2E_DESKTOP=x11
	;;
i3)
	i3 -c /dev/null >/dev/null 2>&1 &
	sleep 2
	I3SOCK=$(i3 --get-socketpath)
	export I3SOCK
	export E2E_FOCUS='i3-msg -q "[title=\"^{title}$\"] focus"'
	export E2E_DESKTOP=i3
	;;
esac
sleep 1
# no shell inside: Ubuntu's bash prompt would retitle the window
xterm -T tock-e2e-alpha -e sleep 600 &
xterm -T tock-e2e-beta -e sleep 600 &
sleep 2
export E2E_TITLES=tock-e2e-alpha,tock-e2e-beta
go test -tags e2e ./e2e -v -count=1
